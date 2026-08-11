package handler

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/cs/purchasing-app/internal/extraction"
	"github.com/cs/purchasing-app/internal/repository"
)

// assembleComparison is where the comparison sheet's arithmetic lives, so it is
// tested directly — no DB, no HTTP. The cases below are the ones that decide
// whether the card tells the truth: which figures a column is allowed to use, what
// happens when a vendor never sent a final quote, and which metrics are safe to
// compute across vendors.

func fp(v float64) *float64 { return &v }

func rawSuggestion(t *testing.T, s extraction.Suggestion) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal suggestion: %v", err)
	}
	return b
}

// quote builds a quotation summary as ListQuotations returns one.
func quote(id int64, vendor string, currency string, total float64, initialDoc, finalDoc *int64, age time.Duration) *repository.Quotation {
	q := &repository.Quotation{
		ID:                         id,
		PurchaseRequestID:          1,
		VendorID:                   id * 10,
		TotalAmount:                total,
		Currency:                   currency,
		Status:                     "received",
		InitialQuotationDocumentID: initialDoc,
		FinalQuotationDocumentID:   finalDoc,
		CreatedAt:                  time.Now().Add(-age),
		Vendor:                     &repository.Vendor{ID: id * 10, Name: vendor},
	}
	if initialDoc != nil {
		q.InitialQuotationDocument = &repository.Document{ID: *initialDoc, Filename: "initial.pdf"}
	}
	if finalDoc != nil {
		q.FinalQuotationDocument = &repository.Document{ID: *finalDoc, Filename: "final.pdf"}
	}
	return q
}

func ext(docID int64, raw json.RawMessage, applied bool) *repository.QuotationExtraction {
	e := &repository.QuotationExtraction{
		ID:         docID,
		DocumentID: docID,
		Status:     repository.ExtractionSucceeded,
		RawJSON:    raw,
	}
	if applied {
		stamp := "2026-08-11T00:00:00Z"
		e.AppliedAt = &stamp
	}
	return e
}

func i64(v int64) *int64 { return &v }

// TestComparisonMetrics: two vendors that both sent a final quote. The grand totals
// are tax-inclusive, the negotiated saving is initial − final, and the cross-vendor
// metrics measure against the highest final quote and the approved budget.
func TestComparisonMetrics(t *testing.T) {
	initialA := rawSuggestion(t, extraction.Suggestion{
		VendorName: "Alpha", Currency: "LKR",
		SubtotalAmount: fp(1000), TotalAmount: fp(1000), TotalIncludesTax: boolp(false),
		Taxes: []extraction.TaxLine{{Label: "VAT", Rate: fp(18), Amount: fp(180)}},
		Items: []extraction.LineItem{{Description: "Camera", Quantity: 10, UnitPrice: 100}},
	})
	finalA := rawSuggestion(t, extraction.Suggestion{
		VendorName: "Alpha", Currency: "LKR",
		SubtotalAmount: fp(900), TotalAmount: fp(1062), TotalIncludesTax: boolp(true),
		Taxes: []extraction.TaxLine{{Label: "VAT", Rate: fp(18), Amount: fp(162)}},
		Items: []extraction.LineItem{{Description: "Camera", Quantity: 10, UnitPrice: 90}},
	})
	initialB := rawSuggestion(t, extraction.Suggestion{
		VendorName: "Beta", Currency: "LKR", TotalAmount: fp(1500),
		Items: []extraction.LineItem{{Description: "Camera", Quantity: 10, UnitPrice: 150}},
	})
	finalB := rawSuggestion(t, extraction.Suggestion{
		VendorName: "Beta", Currency: "LKR", TotalAmount: fp(1400),
		Items: []extraction.LineItem{{Description: "Camera", Quantity: 10, UnitPrice: 140}},
	})

	quotes := []*repository.Quotation{
		quote(2, "Beta", "LKR", 0, i64(21), i64(22), time.Hour), // newest first, as listed
		quote(1, "Alpha", "LKR", 0, i64(11), i64(12), 2*time.Hour),
	}
	exts := []*repository.QuotationExtraction{
		ext(11, initialA, false), ext(12, finalA, true),
		ext(21, initialB, false), ext(22, finalB, true),
	}
	cmp := &repository.QuotationComparison{ApprovedBudget: fp(1100), Currency: "LKR"}

	view := assembleComparison(quotes, nil, exts, cmp)

	if len(view.Vendors) != 2 {
		t.Fatalf("vendors = %d, want 2", len(view.Vendors))
	}
	// Oldest quotation first — the sheet's Vendor A is the first vendor quoted.
	if view.Vendors[0].VendorName != "Alpha" {
		t.Errorf("first vendor = %q, want Alpha", view.Vendors[0].VendorName)
	}
	a, b := view.Vendors[0], view.Vendors[1]

	// Tax was excluded from Alpha's printed initial total, so it is added.
	if got := *a.Initial.GrandTotal; got != 1180 {
		t.Errorf("Alpha initial total = %v, want 1180", got)
	}
	if !a.Initial.AddedTax {
		t.Error("Alpha initial should be flagged as having tax added")
	}
	if got := *a.Final.GrandTotal; got != 1062 {
		t.Errorf("Alpha final total = %v, want 1062", got)
	}
	if got := *a.NegotiatedSaving; got != 118 {
		t.Errorf("Alpha negotiated saving = %v, want 118", got)
	}
	if got := *a.VarianceVsBudget; got != -38 {
		t.Errorf("Alpha variance vs budget = %v, want -38", got)
	}
	// Beta's 1400 is the most expensive final quote.
	if got := *a.SavingVsHighest; got != -338 {
		t.Errorf("Alpha saving vs highest = %v, want -338", got)
	}
	if got := *b.SavingVsHighest; got != 0 {
		t.Errorf("Beta saving vs highest = %v, want 0", got)
	}
	if !a.Lowest || b.Lowest {
		t.Errorf("lowest final should be Alpha (got Alpha=%v Beta=%v)", a.Lowest, b.Lowest)
	}
	if len(view.MissingFinal) != 0 {
		t.Errorf("missing_final = %v, want empty", view.MissingFinal)
	}
	if view.MixedCurrency {
		t.Error("currencies agree; mixed_currency should be false")
	}
	// Items pair up on the description: one row, both sides filled.
	if len(a.Items) != 1 || a.Items[0].InitialUnitPrice == nil || a.Items[0].FinalUnitPrice == nil {
		t.Fatalf("expected one paired item row, got %+v", a.Items)
	}
	if *a.Items[0].InitialUnitPrice != 100 || *a.Items[0].FinalUnitPrice != 90 {
		t.Errorf("paired prices = %v/%v, want 100/90", *a.Items[0].InitialUnitPrice, *a.Items[0].FinalUnitPrice)
	}
}

// TestComparisonMissingFinalQuote: a vendor with no final PDF is reported in
// missing_final, and its initial figures only stand in for a final quote once the
// consent is recorded — labelled as the stand-in they are.
func TestComparisonMissingFinalQuote(t *testing.T) {
	initial := rawSuggestion(t, extraction.Suggestion{
		VendorName: "Alpha", Currency: "USD", TotalAmount: fp(500),
		Items: []extraction.LineItem{{Description: "Rack", Quantity: 1, UnitPrice: 500}},
	})
	quotes := []*repository.Quotation{quote(1, "Alpha", "USD", 0, i64(11), nil, time.Hour)}
	exts := []*repository.QuotationExtraction{ext(11, initial, true)}

	// Without consent the final column stays empty rather than quietly reusing the
	// initial figures.
	view := assembleComparison(quotes, nil, exts, &repository.QuotationComparison{Currency: "USD"})
	if got := view.MissingFinal; len(got) != 1 || got[0] != "Alpha" {
		t.Fatalf("missing_final = %v, want [Alpha]", got)
	}
	if view.Vendors[0].Final.Available {
		t.Error("final column should be unavailable without consent")
	}
	if view.Vendors[0].FinalIsInitial {
		t.Error("final should not be flagged as a stand-in when it wasn't filled")
	}

	// With consent it is filled from the initial quote and says so.
	view = assembleComparison(quotes, nil, exts,
		&repository.QuotationComparison{Currency: "USD", UseInitialForFinal: true})
	v := view.Vendors[0]
	if !v.Final.Available || !v.FinalIsInitial {
		t.Fatalf("expected the initial figures to stand in, got %+v", v.Final)
	}
	if v.Final.Source != sourceInitial {
		t.Errorf("final source = %q, want %q", v.Final.Source, sourceInitial)
	}
	if *v.Final.GrandTotal != 500 || *v.NegotiatedSaving != 0 {
		t.Errorf("stand-in total = %v, saving = %v; want 500 / 0", *v.Final.GrandTotal, *v.NegotiatedSaving)
	}
	// Still listed as missing: the card must keep flagging it.
	if len(view.MissingFinal) != 1 {
		t.Errorf("missing_final = %v, want it to stay flagged", view.MissingFinal)
	}
}

// TestComparisonFallsBackToQuotationRecord: with no extraction at all (the default
// install — extraction is config-gated) the sheet is built from the quotation rows.
func TestComparisonFallsBackToQuotationRecord(t *testing.T) {
	quotes := []*repository.Quotation{
		quote(1, "Alpha", "USD", 750, nil, nil, time.Hour),
		quote(2, "Beta", "USD", 0, nil, nil, 30*time.Minute), // placeholder, no figures
	}
	items := map[int64][]repository.QuotationItem{
		1: {{Description: "Switch", Quantity: 3, UnitPrice: 250}},
	}
	view := assembleComparison(quotes, items, nil,
		&repository.QuotationComparison{Currency: "USD", UseInitialForFinal: true})

	a, b := view.Vendors[0], view.Vendors[1]
	if a.Initial.Source != sourceRecord {
		t.Errorf("source = %q, want %q", a.Initial.Source, sourceRecord)
	}
	if *a.Initial.GrandTotal != 750 || a.Initial.ItemsTotal != 750 {
		t.Errorf("record total = %v / items %v, want 750/750", *a.Initial.GrandTotal, a.Initial.ItemsTotal)
	}
	if a.Initial.ItemsMismatch {
		t.Error("items agree with the total; no mismatch expected")
	}
	// An empty placeholder quotation has no figures — never a 0.00 that reads as free.
	if b.Initial.Available || b.Initial.GrandTotal != nil {
		t.Errorf("empty quotation should carry no figures, got %+v", b.Initial)
	}
	if !a.Lowest {
		t.Error("the only quotation with figures should be the lowest")
	}
}

// TestComparisonMixedCurrency: a quote in another currency is shown but kept out of
// the cross-vendor metrics.
func TestComparisonMixedCurrency(t *testing.T) {
	quotes := []*repository.Quotation{
		quote(1, "Alpha", "USD", 100, nil, nil, time.Hour),
		quote(2, "Beta", "EUR", 50, nil, nil, 30*time.Minute),
	}
	view := assembleComparison(quotes, nil, nil,
		&repository.QuotationComparison{Currency: "USD", UseInitialForFinal: true})

	if !view.MixedCurrency {
		t.Fatal("mixed_currency should be true")
	}
	a, b := view.Vendors[0], view.Vendors[1]
	if a.SavingVsHighest == nil {
		t.Error("the USD quote should still be measured against the others")
	}
	if b.SavingVsHighest != nil || b.VarianceVsBudget != nil {
		t.Errorf("the EUR quote must stay out of cross-vendor metrics, got %+v / %+v",
			b.SavingVsHighest, b.VarianceVsBudget)
	}
	if b.Lowest {
		t.Error("a quote in another currency must not win on price")
	}
}

// TestPairItemsKeepsUnmatchedRows: an item only one side quoted keeps its own row
// rather than being aligned with an unrelated line.
func TestPairItemsKeepsUnmatchedRows(t *testing.T) {
	initial := []comparisonLineItem{
		{Description: "Camera", Quantity: 2, UnitPrice: 100, Amount: 200},
		{Description: "Installation", Quantity: 1, UnitPrice: 50, Amount: 50},
	}
	final := []comparisonLineItem{
		{Description: "  camera ", Quantity: 2, UnitPrice: 90, Amount: 180},
		{Description: "Extended warranty", Quantity: 1, UnitPrice: 30, Amount: 30},
	}
	rows := pairItems(initial, final)
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	if rows[0].InitialUnitPrice == nil || rows[0].FinalUnitPrice == nil {
		t.Error("Camera should pair despite case and whitespace")
	}
	if rows[1].Description != "Installation" || rows[1].FinalAmount != nil {
		t.Errorf("Installation row should have an empty final side, got %+v", rows[1])
	}
	if rows[2].Description != "Extended warranty" || rows[2].InitialAmount != nil {
		t.Errorf("warranty row should have an empty initial side, got %+v", rows[2])
	}
}

func boolp(v bool) *bool { return &v }
