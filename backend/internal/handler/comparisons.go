package handler

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/cs/purchasing-app/internal/extraction"
	"github.com/cs/purchasing-app/internal/middleware"
	"github.com/cs/purchasing-app/internal/model"
	"github.com/cs/purchasing-app/internal/repository"
)

// Quotation comparison — the WSO2 vendor-quote comparison sheet, rendered as a card
// on the purchase request. See docs/quotation-comparison.md.
//
// Every figure is assembled here, server-side, and nothing is stored: a quotation
// edit (or a newly-read final PDF) changes the next read of the comparison rather
// than leaving a stale snapshot. Assembling it on the server rather than in the
// browser is also what lets *approvers* see it — the quotation and extraction
// endpoints are procurement-only, so a legal or budget approver could never build
// this client-side.

// comparisonSource says where a column's figures came from, so the card never
// presents a stand-in as if it were read off the vendor's own paper.
const (
	// sourcePDF: read out of this slot's own PDF (an extraction).
	sourcePDF = "pdf"
	// sourceRecord: the quotation row's stored figures (no extraction for the slot —
	// e.g. a hand-entered quotation, or extraction not configured).
	sourceRecord = "record"
	// sourceInitial: the vendor has no final quote and the user confirmed comparing
	// on its initial figures.
	sourceInitial = "initial"
)

// comparisonMoneyLine is one row of a quote's additional-cost block: a tax line as
// printed (split taxes stay split — CGST + SGST are never summed into "tax") or a
// non-tax charge (shipping, installation, setup).
type comparisonMoneyLine struct {
	Label  string   `json:"label"`
	Rate   *float64 `json:"rate,omitempty"`
	Amount *float64 `json:"amount"`
}

type comparisonLineItem struct {
	Description string  `json:"description"`
	Quantity    float64 `json:"quantity"`
	UnitPrice   float64 `json:"unit_price"`
	Amount      float64 `json:"amount"`
}

// comparisonQuote is one cell-block of the sheet: a single vendor's initial *or*
// final quote, with the line items and additional costs behind its grand total.
type comparisonQuote struct {
	Available  bool   `json:"available"`
	Source     string `json:"source"`
	DocumentID *int64 `json:"document_id,omitempty"`
	Filename   string `json:"filename,omitempty"`
	Currency   string `json:"currency"`

	Items      []comparisonLineItem `json:"items"`
	ItemsTotal float64              `json:"items_total"`

	Subtotal     *float64              `json:"subtotal"`
	Discount     *float64              `json:"discount"`
	Taxes        []comparisonMoneyLine `json:"taxes"`
	TaxTotal     float64               `json:"tax_total"`
	Charges      []comparisonMoneyLine `json:"charges"`
	ChargesTotal float64               `json:"charges_total"`

	// GrandTotal is always tax-inclusive — the amount payable. Nil when the source
	// gave us nothing: a missing figure is blank on the card, never 0.00.
	GrandTotal *float64 `json:"grand_total"`
	// StatedTotal / AddedTax / DerivedTotal explain how GrandTotal was arrived at,
	// so an adjustment to the printed figure is stated rather than silent.
	StatedTotal  *float64 `json:"stated_total"`
	AddedTax     bool     `json:"added_tax"`
	DerivedTotal bool     `json:"derived_total"`
	// ItemsMismatch flags the same non-blocking divergence the quotation cards show:
	// the total doesn't match the line items with this document's own discount,
	// taxes and charges applied.
	ItemsMismatch  bool    `json:"items_mismatch"`
	ValidUntil     *string `json:"valid_until"`
	QuoteReference string  `json:"quote_reference,omitempty"`
	Confidence     string  `json:"confidence,omitempty"`
	// Applied: these are the figures the quotation record itself carries.
	Applied bool `json:"applied"`
}

// comparisonItemRow is one line of a vendor's item table with the initial and final
// quote side by side. Rows are paired on the item description; an item only one of
// the two quotes lists keeps its own row with the other side blank.
type comparisonItemRow struct {
	Description      string   `json:"description"`
	InitialQuantity  *float64 `json:"initial_quantity"`
	InitialUnitPrice *float64 `json:"initial_unit_price"`
	InitialAmount    *float64 `json:"initial_amount"`
	FinalQuantity    *float64 `json:"final_quantity"`
	FinalUnitPrice   *float64 `json:"final_unit_price"`
	FinalAmount      *float64 `json:"final_amount"`
}

// comparisonVendor is one vendor column-pair of the sheet.
type comparisonVendor struct {
	QuotationID int64  `json:"quotation_id"`
	VendorID    int64  `json:"vendor_id"`
	VendorName  string `json:"vendor_name"`
	Notes       string `json:"notes"`
	Status      string `json:"status"`
	Currency    string `json:"currency"`

	Initial comparisonQuote `json:"initial"`
	Final   comparisonQuote `json:"final"`
	// FinalIsInitial: this vendor has no final quote of its own and its initial
	// figures are standing in for one.
	FinalIsInitial bool `json:"final_is_initial"`

	Items []comparisonItemRow `json:"items"`

	// NegotiatedSaving is initial − final (positive = the final quote came down).
	NegotiatedSaving *float64 `json:"negotiated_saving"`
	// VarianceVsBudget is final − approved budget (positive = over budget).
	VarianceVsBudget *float64 `json:"variance_vs_budget"`
	// SavingVsHighest is final − the highest final quote (≤ 0; 0 for that vendor).
	SavingVsHighest *float64 `json:"saving_vs_highest"`
	// Lowest marks the cheapest final quote in the comparison's currency.
	Lowest bool `json:"lowest"`
}

// comparisonView is the whole card.
type comparisonView struct {
	// Exists: a comparison has been generated. When false the figures are empty for
	// everyone but procurement, who is about to generate one.
	Exists     bool                            `json:"exists"`
	Comparison *repository.QuotationComparison `json:"comparison,omitempty"`
	CanManage  bool                            `json:"can_manage"`
	Currency   string                          `json:"currency"`
	// MixedCurrency: the quotations are not all in the comparison's currency, so the
	// cross-vendor metrics are left blank for the odd ones out rather than adding up
	// figures that don't add up.
	MixedCurrency  bool               `json:"mixed_currency"`
	ApprovedBudget *float64           `json:"approved_budget"`
	Vendors        []comparisonVendor `json:"vendors"`
	// MissingFinal names the vendors whose final quote could not be read from a PDF
	// of its own — what the confirmation modal warns about before generating, and
	// what the card keeps flagging afterwards.
	MissingFinal   []string `json:"missing_final"`
	QuotationCount int      `json:"quotation_count"`
}

// --- endpoints ---

// GetQuotationComparison returns the comparison card for a PR. Readable by anyone
// who may view the request — the comparison exists to be read by the people
// approving the recommendation, who have no access to the quotation endpoints.
//
// Always computes the figures for procurement (who needs the confirmation modal's
// missing-final list *before* generating); for everyone else the figures are
// returned only once a comparison has actually been generated.
func (h *PurchaseRequestsHandler) GetQuotationComparison(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.loadViewablePR(w, r)
	if !ok {
		return
	}
	view, ok := h.buildComparison(w, r, pr)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, view)
}

type comparisonInput struct {
	ApprovedBudget     *float64 `json:"approved_budget"`
	Currency           string   `json:"currency"`
	UseInitialForFinal bool     `json:"use_initial_for_final"`
}

func (in comparisonInput) toRepo() repository.QuotationComparisonInput {
	return repository.QuotationComparisonInput{
		ApprovedBudget:     in.ApprovedBudget,
		Currency:           strings.ToUpper(strings.TrimSpace(in.Currency)),
		UseInitialForFinal: in.UseInitialForFinal,
	}
}

// GenerateQuotationComparison creates (or regenerates) the PR's comparison. It
// needs two or more quotations — a comparison of one is not a comparison — and the
// caller's confirmation when any vendor's final quote is missing, since standing
// its initial figures in for one is an assumption the user has to make knowingly.
func (h *PurchaseRequestsHandler) GenerateQuotationComparison(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.loadComparisonWorkPR(w, r)
	if !ok {
		return
	}
	var in comparisonInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	repoIn := in.toRepo()

	view, ok := h.buildComparison(w, r, pr)
	if !ok {
		return
	}
	if view.QuotationCount < 2 {
		writeError(w, http.StatusConflict, "at least two quotations are needed to compare")
		return
	}
	if len(view.MissingFinal) > 0 && !repoIn.UseInitialForFinal {
		writeError(w, http.StatusConflict,
			"no final quotation for "+strings.Join(view.MissingFinal, ", ")+
				" — confirm comparing on their initial figures")
		return
	}
	if repoIn.Currency == "" {
		repoIn.Currency = view.Currency
	}

	existing := view.Comparison
	cmp, err := h.Repo.GenerateQuotationComparison(r.Context(), pr.ID, repoIn, middleware.UserFromCtx(r.Context()).ID)
	if err != nil {
		reqLog(r).Error().Err(err).Int64("pr_id", pr.ID).Msg("generate quotation comparison")
		writeError(w, http.StatusInternalServerError, "failed to generate the comparison")
		return
	}
	action := model.ProcessCreateQuotationComparison
	if existing != nil {
		action = model.ProcessUpdateQuotationComparison
	}
	recordProcessEvent(r, h.Repo, pr.ID, action, "")
	ensureCollaborator(r, h.Repo, pr.ID)

	out, ok := h.buildComparison(w, r, pr)
	if !ok {
		return
	}
	out.Comparison = cmp
	out.Exists = true
	writeJSON(w, http.StatusOK, out)
}

// UpdateQuotationComparison edits the comparison's own inputs — the approved budget
// it measures against, the currency it is stated in, and the missing-final consent.
// The quotation figures are never edited here; they belong to the quotations.
func (h *PurchaseRequestsHandler) UpdateQuotationComparison(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.loadComparisonWorkPR(w, r)
	if !ok {
		return
	}
	var in comparisonInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	cmp, err := h.Repo.UpdateQuotationComparison(r.Context(), pr.ID, in.toRepo())
	if err != nil {
		reqLog(r).Error().Err(err).Int64("pr_id", pr.ID).Msg("update quotation comparison")
		writeError(w, http.StatusInternalServerError, "failed to update the comparison")
		return
	}
	if cmp == nil {
		writeError(w, http.StatusNotFound, "no comparison has been generated for this request")
		return
	}
	recordProcessEvent(r, h.Repo, pr.ID, model.ProcessUpdateQuotationComparison, "")
	ensureCollaborator(r, h.Repo, pr.ID)

	view, ok := h.buildComparison(w, r, pr)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// DeleteQuotationComparison discards the comparison. The quotations it compared are
// untouched, so generating it again rebuilds the same sheet from current figures.
func (h *PurchaseRequestsHandler) DeleteQuotationComparison(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.loadComparisonWorkPR(w, r)
	if !ok {
		return
	}
	if err := h.Repo.DeleteQuotationComparison(r.Context(), pr.ID); err != nil {
		reqLog(r).Error().Err(err).Int64("pr_id", pr.ID).Msg("delete quotation comparison")
		writeError(w, http.StatusInternalServerError, "failed to remove the comparison")
		return
	}
	recordProcessEvent(r, h.Repo, pr.ID, model.ProcessDeleteQuotationComparison, "")
	ensureCollaborator(r, h.Repo, pr.ID)
	w.WriteHeader(http.StatusNoContent)
}

// loadComparisonWorkPR loads the PR and applies the same gates as every other piece
// of procurement work on it: procurement access, team-lead approval, assignment.
func (h *PurchaseRequestsHandler) loadComparisonWorkPR(w http.ResponseWriter, r *http.Request) (*repository.PurchaseRequest, bool) {
	if !middleware.HasProcurementAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "procurement access required")
		return nil, false
	}
	pr, ok := h.load(w, r)
	if !ok {
		return nil, false
	}
	if !model.IsTeamLeadApproved(pr.TeamLeadStatus) {
		writeError(w, http.StatusConflict, "purchase request is awaiting team lead approval")
		return nil, false
	}
	if code, msg, ok := assignmentWorkGate(r, pr); !ok {
		writeError(w, code, msg)
		return nil, false
	}
	return pr, true
}

// buildComparison loads what the sheet is derived from and assembles it. Returns
// false having already written an error response.
func (h *PurchaseRequestsHandler) buildComparison(w http.ResponseWriter, r *http.Request, pr *repository.PurchaseRequest) (comparisonView, bool) {
	ctx := r.Context()
	cmp, err := h.Repo.GetQuotationComparison(ctx, pr.ID)
	if err != nil {
		reqLog(r).Error().Err(err).Int64("pr_id", pr.ID).Msg("load quotation comparison")
		writeError(w, http.StatusInternalServerError, "failed to load the comparison")
		return comparisonView{}, false
	}
	canManage := middleware.HasProcurementAccess(ctx) &&
		model.IsTeamLeadApproved(pr.TeamLeadStatus) && pr.AssigneeID != nil
	// Nothing generated and the caller isn't the one who would generate it: there is
	// no comparison to show, and the quotation figures are not theirs to browse.
	if cmp == nil && !canManage {
		return comparisonView{CanManage: false, Vendors: []comparisonVendor{}, MissingFinal: []string{}}, true
	}

	quotes, err := h.Repo.ListQuotations(ctx, &pr.ID)
	if err != nil {
		reqLog(r).Error().Err(err).Int64("pr_id", pr.ID).Msg("comparison: list quotations")
		writeError(w, http.StatusInternalServerError, "failed to load quotations")
		return comparisonView{}, false
	}
	items, err := h.Repo.QuotationItemsForPR(ctx, pr.ID)
	if err != nil {
		reqLog(r).Error().Err(err).Int64("pr_id", pr.ID).Msg("comparison: list quotation items")
		writeError(w, http.StatusInternalServerError, "failed to load quotation items")
		return comparisonView{}, false
	}
	exts, err := h.Repo.ListSucceededExtractionsForPR(ctx, pr.ID)
	if err != nil {
		reqLog(r).Error().Err(err).Int64("pr_id", pr.ID).Msg("comparison: list extractions")
		writeError(w, http.StatusInternalServerError, "failed to load extracted quotation details")
		return comparisonView{}, false
	}

	view := assembleComparison(quotes, items, exts, cmp)
	view.CanManage = canManage
	return view, true
}

// --- derivation (pure; unit-tested in comparisons_test.go) ---

// assembleComparison turns a PR's quotations, their stored items and whatever was
// read out of their PDFs into the comparison sheet.
//
// Each vendor gets two columns. A column's figures come from that PDF's own
// extraction where there is one; failing that from the quotation record (which is
// all there is when extraction isn't configured); and for a final quote that
// doesn't exist, from the initial quote — but only with the user's recorded consent,
// and always labelled as the stand-in it is.
func assembleComparison(
	quotes []*repository.Quotation,
	items map[int64][]repository.QuotationItem,
	exts []*repository.QuotationExtraction,
	cmp *repository.QuotationComparison,
) comparisonView {
	view := comparisonView{
		Exists:         cmp != nil,
		Comparison:     cmp,
		Vendors:        []comparisonVendor{},
		MissingFinal:   []string{},
		QuotationCount: len(quotes),
	}
	if cmp != nil {
		view.ApprovedBudget = cmp.ApprovedBudget
		view.Currency = cmp.Currency
	}

	// Oldest first: the sheet's Vendor A / B / C are the order they were quoted in,
	// while ListQuotations answers newest-first for the activity-style lists.
	ordered := make([]*repository.Quotation, len(quotes))
	copy(ordered, quotes)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].CreatedAt.Before(ordered[j].CreatedAt) })

	byDoc := map[int64]*extraction.Suggestion{}
	appliedDoc := map[int64]bool{}
	for _, e := range exts {
		if e.Status != repository.ExtractionSucceeded || len(e.RawJSON) == 0 {
			continue
		}
		var sug extraction.Suggestion
		if err := json.Unmarshal(e.RawJSON, &sug); err != nil {
			continue // an unreadable stored read is the same as no read
		}
		byDoc[e.DocumentID] = &sug
		if e.AppliedAt != nil {
			appliedDoc[e.DocumentID] = true
		}
	}

	useInitial := cmp != nil && cmp.UseInitialForFinal
	for _, q := range ordered {
		v := comparisonVendor{
			QuotationID: q.ID,
			VendorID:    q.VendorID,
			VendorName:  vendorLabel(q),
			Notes:       q.Notes,
			Status:      q.Status,
			Currency:    q.Currency,
		}
		initialSug := suggestionFor(byDoc, q.InitialQuotationDocumentID)
		finalSug := suggestionFor(byDoc, q.FinalQuotationDocumentID)
		finalApplied := q.FinalQuotationDocumentID != nil && appliedDoc[*q.FinalQuotationDocumentID]

		switch {
		case initialSug != nil:
			v.Initial = quoteFromSuggestion(*initialSug, q, q.InitialQuotationDocumentID, q.InitialQuotationDocument)
			v.Initial.Applied = q.InitialQuotationDocumentID != nil && appliedDoc[*q.InitialQuotationDocumentID]
		case !finalApplied:
			// No read of the initial PDF, but the record's figures aren't the final
			// PDF's either, so they are the best statement of the initial quote.
			v.Initial = quoteFromRecord(q, items[q.ID])
		}

		switch {
		case finalSug != nil:
			v.Final = quoteFromSuggestion(*finalSug, q, q.FinalQuotationDocumentID, q.FinalQuotationDocument)
			v.Final.Applied = finalApplied
		case finalApplied:
			v.Final = quoteFromRecord(q, items[q.ID])
		}

		if !v.Final.Available {
			view.MissingFinal = append(view.MissingFinal, v.VendorName)
			if useInitial && v.Initial.Available {
				v.Final = v.Initial
				v.Final.Source = sourceInitial
				v.FinalIsInitial = true
			}
		}

		v.Items = pairItems(v.Initial.Items, v.Final.Items)
		if v.Initial.GrandTotal != nil && v.Final.GrandTotal != nil {
			d := extraction.Round2(*v.Initial.GrandTotal - *v.Final.GrandTotal)
			v.NegotiatedSaving = &d
		}
		view.Vendors = append(view.Vendors, v)
	}

	// The comparison's currency: what it was generated in, else the currency the
	// quotations agree on (the first one's, when they don't).
	if view.Currency == "" && len(view.Vendors) > 0 {
		view.Currency = view.Vendors[0].Currency
	}
	for _, v := range view.Vendors {
		if v.Currency != "" && v.Currency != view.Currency {
			view.MixedCurrency = true
		}
	}

	// Cross-vendor metrics, over the vendors actually stated in the comparison's
	// currency — comparing a figure in one currency against another's isn't a
	// comparison, it's a mistake with a number on it.
	var highest *float64
	var lowestIdx = -1
	for i, v := range view.Vendors {
		if v.Final.GrandTotal == nil || !comparable(v, view.Currency) {
			continue
		}
		if highest == nil || *v.Final.GrandTotal > *highest {
			highest = v.Final.GrandTotal
		}
		if lowestIdx < 0 || *v.Final.GrandTotal < *view.Vendors[lowestIdx].Final.GrandTotal {
			lowestIdx = i
		}
	}
	for i := range view.Vendors {
		v := &view.Vendors[i]
		if v.Final.GrandTotal == nil || !comparable(*v, view.Currency) {
			continue
		}
		if view.ApprovedBudget != nil {
			d := extraction.Round2(*v.Final.GrandTotal - *view.ApprovedBudget)
			v.VarianceVsBudget = &d
		}
		if highest != nil {
			d := extraction.Round2(*v.Final.GrandTotal - *highest)
			v.SavingVsHighest = &d
		}
	}
	if lowestIdx >= 0 && len(view.Vendors) > 1 {
		view.Vendors[lowestIdx].Lowest = true
	}
	return view
}

// comparable reports whether a vendor's figures may be measured against the other
// vendors' — i.e. they are in the comparison's currency.
func comparable(v comparisonVendor, currency string) bool {
	return v.Currency == "" || currency == "" || v.Currency == currency
}

func vendorLabel(q *repository.Quotation) string {
	if q.Vendor != nil && q.Vendor.Name != "" {
		return q.Vendor.Name
	}
	return "Vendor"
}

func suggestionFor(byDoc map[int64]*extraction.Suggestion, docID *int64) *extraction.Suggestion {
	if docID == nil {
		return nil
	}
	return byDoc[*docID]
}

// quoteFromSuggestion builds a column from what was read out of one PDF — the rich
// case: the totals block as printed, with split taxes kept split.
func quoteFromSuggestion(s extraction.Suggestion, q *repository.Quotation, docID *int64, doc *repository.Document) comparisonQuote {
	taxTotal := s.TaxTotal()
	reading := extraction.TaxInclusiveTotal(s, taxTotal)
	out := comparisonQuote{
		Available:      true,
		Source:         sourcePDF,
		DocumentID:     docID,
		Currency:       firstNonEmpty(s.Currency, q.Currency),
		Subtotal:       s.SubtotalAmount,
		Discount:       s.DiscountAmount,
		TaxTotal:       extraction.Round2(taxTotal),
		ChargesTotal:   extraction.Round2(s.ChargesTotal()),
		GrandTotal:     reading.Value,
		StatedTotal:    reading.Stated,
		AddedTax:       reading.AddedTax,
		DerivedTotal:   reading.Derived,
		ValidUntil:     s.ValidUntil,
		QuoteReference: s.QuoteReference,
		Confidence:     s.Confidence,
		Taxes:          []comparisonMoneyLine{},
		Charges:        []comparisonMoneyLine{},
		Items:          []comparisonLineItem{},
	}
	if doc != nil {
		out.Filename = doc.Filename
	}
	for _, t := range s.Taxes {
		out.Taxes = append(out.Taxes, comparisonMoneyLine{Label: orDefault(t.Label, "Tax"), Rate: t.Rate, Amount: t.Amount})
	}
	for _, c := range s.OtherCharges {
		out.Charges = append(out.Charges, comparisonMoneyLine{Label: orDefault(c.Label, "Charge"), Amount: c.Amount})
	}
	for _, it := range s.Items {
		out.Items = append(out.Items, comparisonLineItem{
			Description: it.Description,
			Quantity:    it.Quantity,
			UnitPrice:   it.UnitPrice,
			Amount:      extraction.Round2(it.Quantity * it.UnitPrice),
		})
	}
	out.ItemsTotal = extraction.Round2(s.ItemsTotal())
	if len(out.Items) > 0 && reading.Value != nil {
		built := extraction.ReconciledTotal(s, out.ItemsTotal, taxTotal)
		out.ItemsMismatch = absDiff(*reading.Value, built) > 0.01
	}
	return out
}

// quoteFromRecord builds a column from the quotation row itself — what a
// hand-entered quotation (or an install with extraction switched off) has. There is
// no totals breakdown to show: the row stores one tax-inclusive total.
func quoteFromRecord(q *repository.Quotation, stored []repository.QuotationItem) comparisonQuote {
	out := comparisonQuote{
		Source:     sourceRecord,
		Currency:   q.Currency,
		ValidUntil: q.ValidUntil,
		Taxes:      []comparisonMoneyLine{},
		Charges:    []comparisonMoneyLine{},
		Items:      []comparisonLineItem{},
	}
	for _, it := range stored {
		out.Items = append(out.Items, comparisonLineItem{
			Description: it.Description,
			Quantity:    it.Quantity,
			UnitPrice:   it.UnitPrice,
			Amount:      extraction.Round2(it.Quantity * it.UnitPrice),
		})
		out.ItemsTotal = extraction.Round2(out.ItemsTotal + it.Quantity*it.UnitPrice)
	}
	// A quotation associated inline on the PR page starts at 0 with no items: that
	// is an empty placeholder, not a free quote, so it stays blank.
	if q.TotalAmount == 0 && len(out.Items) == 0 {
		return out
	}
	out.Available = true
	if q.TotalAmount != 0 {
		total := extraction.Round2(q.TotalAmount)
		out.GrandTotal = &total
		out.StatedTotal = &total
		if len(out.Items) > 0 {
			out.ItemsMismatch = absDiff(total, out.ItemsTotal) > 0.01
		}
	} else {
		total := out.ItemsTotal
		out.GrandTotal = &total
		out.DerivedTotal = true
	}
	return out
}

// pairItems lines the initial and final quote's items up on their descriptions, so a
// row shows what a vendor charged for the same thing before and after negotiation.
// An item only one side lists keeps its own row with the other side blank — never
// forced onto an unrelated line by position.
func pairItems(initial, final []comparisonLineItem) []comparisonItemRow {
	rows := []comparisonItemRow{}
	used := make([]bool, len(final))
	for _, it := range initial {
		row := comparisonItemRow{Description: it.Description}
		setInitial(&row, it)
		for j, f := range final {
			if used[j] || normalizeItemKey(f.Description) != normalizeItemKey(it.Description) {
				continue
			}
			used[j] = true
			setFinal(&row, f)
			break
		}
		rows = append(rows, row)
	}
	for j, f := range final {
		if used[j] {
			continue
		}
		row := comparisonItemRow{Description: f.Description}
		setFinal(&row, f)
		rows = append(rows, row)
	}
	return rows
}

func setInitial(row *comparisonItemRow, it comparisonLineItem) {
	qty, price, amt := it.Quantity, it.UnitPrice, it.Amount
	row.InitialQuantity, row.InitialUnitPrice, row.InitialAmount = &qty, &price, &amt
}

func setFinal(row *comparisonItemRow, it comparisonLineItem) {
	qty, price, amt := it.Quantity, it.UnitPrice, it.Amount
	row.FinalQuantity, row.FinalUnitPrice, row.FinalAmount = &qty, &price, &amt
}

// normalizeItemKey collapses case and whitespace so "Firewall  Appliance" and
// "firewall appliance" pair up. Deliberately conservative — it never pairs items
// that merely look similar.
func normalizeItemKey(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

func orDefault(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

func absDiff(a, b float64) float64 {
	if a > b {
		return a - b
	}
	return b - a
}
