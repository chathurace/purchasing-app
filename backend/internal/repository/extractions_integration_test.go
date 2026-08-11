package repository_test

import (
	"context"
	"testing"

	"github.com/cs/purchasing-app/internal/model"
	"github.com/cs/purchasing-app/internal/repository"
)

// cleanupPR removes everything a test created, in FK-safe order: the purchase
// request (which cascades to its documents, quotations and extractions), then the
// master-data rows that do not cascade — vendors, then the user.
//
// Registered as a single t.Cleanup so ordering is explicit; t.Cleanup itself runs
// LIFO, which makes multiple independent cleanups easy to get wrong (a vendor
// delete scheduled after the PR delete runs first and trips the quotation's FK).
// Errors are reported, not swallowed, so a silent leak can't hide here.
func cleanupPR(t *testing.T, repo *repository.Repository, ctx context.Context, prID, userID int64, vendorIDs ...int64) {
	t.Helper()
	t.Cleanup(func() {
		if _, err := repo.Pool().Exec(ctx, `DELETE FROM purchase_requests WHERE id=$1`, prID); err != nil {
			t.Errorf("cleanup purchase request %d: %v", prID, err)
		}
		if len(vendorIDs) > 0 {
			if _, err := repo.Pool().Exec(ctx, `DELETE FROM vendors WHERE id = ANY($1)`, vendorIDs); err != nil {
				t.Errorf("cleanup vendors %v: %v", vendorIDs, err)
			}
		}
		if _, err := repo.Pool().Exec(ctx, `DELETE FROM users WHERE id=$1`, userID); err != nil {
			t.Errorf("cleanup user %d: %v", userID, err)
		}
	})
}

// TestQuotationExtractionFlow exercises the staging table end to end against the
// real schema: stage → finish → adopt onto a quotation, plus the re-run and
// guard paths. This is where the hand-written SQL in extractions.go is verified.
func TestQuotationExtractionFlow(t *testing.T) {
	repo, ctx := newTestRepo(t)

	user, err := repo.UpsertUser(ctx, "extract-sub-"+t.Name(), "extract@example.com", "Extract Tester")
	if err != nil {
		t.Fatalf("upsert user: %v", err)
	}
	pr, err := repo.CreatePurchaseRequest(ctx, user.ID, repository.PurchaseRequestInput{Title: "Extraction test"})
	if err != nil {
		t.Fatalf("create PR: %v", err)
	}
	// A staging document, as ExtractForPR stores it before the quotation exists.
	doc, err := repo.AddOwnedDocument(ctx, pr.ID, model.OwnerQuotationExtraction, 0,
		"vendor-quote.pdf", "test/vendor-quote.pdf", "application/pdf", 4096, user.ID, "")
	if err != nil {
		t.Fatalf("add staging document: %v", err)
	}

	// --- stage ---
	ext, err := repo.CreateExtraction(ctx, pr.ID, doc.ID, nil, "claude-opus-5", user.ID)
	if err != nil {
		t.Fatalf("create extraction: %v", err)
	}
	if ext.Status != repository.ExtractionPending {
		t.Fatalf("new extraction status = %q, want pending", ext.Status)
	}
	if ext.QuotationID != nil {
		t.Fatalf("pre-create extraction has quotation_id %v, want nil", *ext.QuotationID)
	}
	if ext.Filename != "vendor-quote.pdf" {
		t.Errorf("Filename = %q, want the joined document filename", ext.Filename)
	}

	// A pending extraction must not be adoptable — only a succeeded one is.
	if _, err := repo.TakePendingExtractionDocument(ctx, ext.ID, pr.ID); err != repository.ErrInvalidState {
		t.Fatalf("adopting a pending extraction = %v, want ErrInvalidState", err)
	}

	// --- finish ---
	raw := []byte(`{"vendor_name":"Acme Supplies","currency":"USD","total_amount":990.5,"items":[]}`)
	if err := repo.FinishExtraction(ctx, ext.ID, repository.ExtractionSucceeded, raw, 1200, 340, ""); err != nil {
		t.Fatalf("finish extraction: %v", err)
	}
	ext, err = repo.GetExtraction(ctx, ext.ID)
	if err != nil {
		t.Fatalf("reload extraction: %v", err)
	}
	if ext.Status != repository.ExtractionSucceeded {
		t.Fatalf("status = %q, want succeeded", ext.Status)
	}
	if ext.InputTokens != 1200 || ext.OutputTokens != 340 {
		t.Errorf("tokens = %d/%d, want 1200/340", ext.InputTokens, ext.OutputTokens)
	}
	if len(ext.RawJSON) == 0 {
		t.Fatal("raw_json did not round-trip through jsonb")
	}

	// Wrong PR must be rejected even for a succeeded row.
	if _, err := repo.TakePendingExtractionDocument(ctx, ext.ID, pr.ID+99999); err != repository.ErrInvalidState {
		t.Fatalf("adopting across PRs = %v, want ErrInvalidState", err)
	}

	// --- adopt onto a quotation (what quotation-create does) ---
	vendor, err := repo.CreateVendor(ctx, repository.VendorInput{Name: "Acme Supplies Ltd"}, user.ID)
	if err != nil {
		t.Fatalf("create vendor: %v", err)
	}
	cleanupPR(t, repo, ctx, pr.ID, user.ID, vendor.ID)
	quo, err := repo.CreateQuotation(ctx, pr.ID, repository.QuotationInput{
		VendorID: vendor.ID, TotalAmount: 990.5, Currency: "USD",
	}, user.ID)
	if err != nil {
		t.Fatalf("create quotation: %v", err)
	}

	docID, err := repo.TakePendingExtractionDocument(ctx, ext.ID, pr.ID)
	if err != nil {
		t.Fatalf("take extraction document: %v", err)
	}
	if docID != doc.ID {
		t.Fatalf("adopted document %d, want %d", docID, doc.ID)
	}
	if err := repo.SetDocumentOwner(ctx, docID, model.OwnerQuotation, quo.ID); err != nil {
		t.Fatalf("set document owner: %v", err)
	}
	if _, err := repo.SetQuotationDocument(ctx, quo.ID, repository.QuotationDocInitial, docID); err != nil {
		t.Fatalf("set quotation document: %v", err)
	}
	if err := repo.MarkExtractionApplied(ctx, ext.ID, quo.ID, user.ID); err != nil {
		t.Fatalf("mark applied: %v", err)
	}

	// The re-owned document must now be reachable as the quotation's initial PDF.
	reloaded, err := repo.GetQuotation(ctx, quo.ID)
	if err != nil {
		t.Fatalf("reload quotation: %v", err)
	}
	if reloaded.InitialQuotationDocument == nil {
		t.Fatal("initial_quotation_document is nil after adoption")
	}
	if reloaded.InitialQuotationDocument.Filename != "vendor-quote.pdf" {
		t.Errorf("adopted filename = %q", reloaded.InitialQuotationDocument.Filename)
	}

	// An applied extraction must not be adoptable a second time.
	if _, err := repo.TakePendingExtractionDocument(ctx, ext.ID, pr.ID); err != repository.ErrInvalidState {
		t.Fatalf("re-adopting an applied extraction = %v, want ErrInvalidState", err)
	}

	ext, err = repo.GetExtraction(ctx, ext.ID)
	if err != nil {
		t.Fatalf("reload applied extraction: %v", err)
	}
	if ext.AppliedAt == nil {
		t.Error("applied_at not stamped")
	}
	if ext.QuotationID == nil || *ext.QuotationID != quo.ID {
		t.Errorf("quotation_id = %v, want %d", ext.QuotationID, quo.ID)
	}
}

// TestCreateExtractionReplacesPriorAttempt covers the unique-per-document index:
// re-running extraction on the same PDF must replace the live row rather than
// erroring on the constraint.
func TestCreateExtractionReplacesPriorAttempt(t *testing.T) {
	repo, ctx := newTestRepo(t)

	user, err := repo.UpsertUser(ctx, "extract-rerun-"+t.Name(), "rerun@example.com", "Rerun Tester")
	if err != nil {
		t.Fatalf("upsert user: %v", err)
	}
	pr, err := repo.CreatePurchaseRequest(ctx, user.ID, repository.PurchaseRequestInput{Title: "Re-extract"})
	if err != nil {
		t.Fatalf("create PR: %v", err)
	}
	cleanupPR(t, repo, ctx, pr.ID, user.ID)

	doc, err := repo.AddOwnedDocument(ctx, pr.ID, model.OwnerQuotationExtraction, 0,
		"q.pdf", "test/q.pdf", "application/pdf", 100, user.ID, "")
	if err != nil {
		t.Fatalf("add document: %v", err)
	}

	first, err := repo.CreateExtraction(ctx, pr.ID, doc.ID, nil, "claude-opus-5", user.ID)
	if err != nil {
		t.Fatalf("first extraction: %v", err)
	}
	second, err := repo.CreateExtraction(ctx, pr.ID, doc.ID, nil, "claude-opus-5", user.ID)
	if err != nil {
		t.Fatalf("second extraction on the same document: %v", err)
	}
	if second.ID == first.ID {
		t.Fatal("re-extraction returned the same row id")
	}
	if _, err := repo.GetExtraction(ctx, first.ID); err == nil {
		t.Error("the superseded extraction row still exists")
	}

	// A failed attempt is retained (it records the failure) and must not block a retry.
	if err := repo.FinishExtraction(ctx, second.ID, repository.ExtractionFailed, nil, 0, 0, "boom"); err != nil {
		t.Fatalf("fail extraction: %v", err)
	}
	third, err := repo.CreateExtraction(ctx, pr.ID, doc.ID, nil, "claude-opus-5", user.ID)
	if err != nil {
		t.Fatalf("retry after failure: %v", err)
	}
	if _, err := repo.GetExtraction(ctx, second.ID); err != nil {
		t.Error("the failed attempt was deleted; it should be kept as a record")
	}
	if third.Status != repository.ExtractionPending {
		t.Errorf("retry status = %q, want pending", third.Status)
	}
}

// TestMatchVendors checks the ranking a reviewer sees for an extracted vendor name.
func TestMatchVendors(t *testing.T) {
	repo, ctx := newTestRepo(t)

	user, err := repo.UpsertUser(ctx, "match-sub-"+t.Name(), "match@example.com", "Match Tester")
	if err != nil {
		t.Fatalf("upsert user: %v", err)
	}
	// A distinctive stem keeps this test independent of other rows in the dev DB.
	exact, err := repo.CreateVendor(ctx, repository.VendorInput{Name: "Zyxwq Software Solutions, Ltda."}, user.ID)
	if err != nil {
		t.Fatalf("create vendor: %v", err)
	}
	related, err := repo.CreateVendor(ctx, repository.VendorInput{Name: "Zyxwq Brasil"}, user.ID)
	if err != nil {
		t.Fatalf("create related vendor: %v", err)
	}
	t.Cleanup(func() {
		if _, err := repo.Pool().Exec(ctx, `DELETE FROM vendors WHERE id = ANY($1)`,
			[]int64{exact.ID, related.ID}); err != nil {
			t.Errorf("cleanup vendors: %v", err)
		}
		if _, err := repo.Pool().Exec(ctx, `DELETE FROM users WHERE id=$1`, user.ID); err != nil {
			t.Errorf("cleanup user: %v", err)
		}
	})

	// Punctuation and legal-form differences must still score as an exact match.
	matches, err := repo.MatchVendors(ctx, "zyxwq software solutions ltda", 5)
	if err != nil {
		t.Fatalf("match vendors: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("no matches for an exactly-named vendor")
	}
	if matches[0].Vendor.ID != exact.ID {
		t.Errorf("best match = %q, want %q", matches[0].Vendor.Name, exact.Name)
	}
	if matches[0].Score != 100 {
		t.Errorf("best score = %d, want 100 for an exact match", matches[0].Score)
	}

	// A shared significant word should surface the sibling vendor too.
	matches, err = repo.MatchVendors(ctx, "Zyxwq", 5)
	if err != nil {
		t.Fatalf("match vendors by shared word: %v", err)
	}
	ids := map[int64]bool{}
	for _, m := range matches {
		ids[m.Vendor.ID] = true
	}
	if !ids[exact.ID] || !ids[related.ID] {
		t.Errorf("matching %q returned %d rows; want both Zyxwq vendors", "Zyxwq", len(matches))
	}

	// An unrelated name matches nothing, so the UI offers to create a vendor.
	matches, err = repo.MatchVendors(ctx, "Qqzz Unrelated Holdings", 5)
	if err != nil {
		t.Fatalf("match unrelated: %v", err)
	}
	for _, m := range matches {
		if m.Vendor.ID == exact.ID || m.Vendor.ID == related.ID {
			t.Errorf("unrelated name matched %q", m.Vendor.Name)
		}
	}

	// An empty extracted name must not scan the whole vendor list.
	if got, err := repo.MatchVendors(ctx, "   ", 5); err != nil || len(got) != 0 {
		t.Errorf("MatchVendors(blank) = %v, %v; want no matches", got, err)
	}
}

// TestApplyingOneExtractionUnappliesTheOther pins the invariant behind the
// applied/not-applied chip on a quotation card: a quotation stores one total, so at
// most one of its PDFs' reads can be the one it carries.
//
// The initial and final quotation PDFs are separate documents with separate
// extractions. Applying the final one's figures must clear the initial one's stamp —
// otherwise both cards claim "applied" and the chip stops answering the only question
// it exists to answer.
func TestApplyingOneExtractionUnappliesTheOther(t *testing.T) {
	repo, ctx := newTestRepo(t)

	user, err := repo.UpsertUser(ctx, "extract-excl-"+t.Name(), "excl@example.com", "Exclusive Tester")
	if err != nil {
		t.Fatalf("upsert user: %v", err)
	}
	pr, err := repo.CreatePurchaseRequest(ctx, user.ID, repository.PurchaseRequestInput{Title: "Applied exclusivity"})
	if err != nil {
		t.Fatalf("create PR: %v", err)
	}
	vendor, err := repo.CreateVendor(ctx, repository.VendorInput{Name: "Exclusive Vendor Ltd"}, user.ID)
	if err != nil {
		t.Fatalf("create vendor: %v", err)
	}
	cleanupPR(t, repo, ctx, pr.ID, user.ID, vendor.ID)

	quo, err := repo.CreateQuotation(ctx, pr.ID, repository.QuotationInput{
		VendorID: vendor.ID, TotalAmount: 100, Currency: "USD",
	}, user.ID)
	if err != nil {
		t.Fatalf("create quotation: %v", err)
	}

	// Two PDFs on the same quotation, each read once.
	stage := func(name string) *repository.QuotationExtraction {
		t.Helper()
		doc, err := repo.AddOwnedDocument(ctx, pr.ID, model.OwnerQuotation, quo.ID,
			name, "test/"+name, "application/pdf", 2048, user.ID, "")
		if err != nil {
			t.Fatalf("add document %s: %v", name, err)
		}
		ext, err := repo.CreateExtraction(ctx, pr.ID, doc.ID, &quo.ID, "claude-opus-5", user.ID)
		if err != nil {
			t.Fatalf("create extraction for %s: %v", name, err)
		}
		if err := repo.FinishExtraction(ctx, ext.ID, repository.ExtractionSucceeded,
			[]byte(`{"vendor_name":"Exclusive Vendor","items":[]}`), 10, 10, ""); err != nil {
			t.Fatalf("finish extraction for %s: %v", name, err)
		}
		return ext
	}
	initial := stage("initial.pdf")
	final := stage("final.pdf")

	applied := func(id int64) bool {
		t.Helper()
		ext, err := repo.GetExtraction(ctx, id)
		if err != nil {
			t.Fatalf("reload extraction %d: %v", id, err)
		}
		return ext.AppliedAt != nil
	}

	// Apply the initial read.
	if err := repo.MarkExtractionApplied(ctx, initial.ID, quo.ID, user.ID); err != nil {
		t.Fatalf("mark initial applied: %v", err)
	}
	if !applied(initial.ID) {
		t.Fatal("initial extraction not stamped applied")
	}
	if applied(final.ID) {
		t.Fatal("final extraction is applied without being applied")
	}

	// Then apply the final read — the post-negotiation numbers supersede.
	if err := repo.MarkExtractionApplied(ctx, final.ID, quo.ID, user.ID); err != nil {
		t.Fatalf("mark final applied: %v", err)
	}
	if !applied(final.ID) {
		t.Error("final extraction not stamped applied")
	}
	if applied(initial.ID) {
		t.Error("initial extraction is still applied; two PDFs both claim the quotation's figures")
	}

	// An extraction on a *different* quotation must be untouched by all of this.
	other, err := repo.CreateQuotation(ctx, pr.ID, repository.QuotationInput{
		VendorID: vendor.ID, TotalAmount: 200, Currency: "USD",
	}, user.ID)
	if err != nil {
		t.Fatalf("create second quotation: %v", err)
	}
	otherDoc, err := repo.AddOwnedDocument(ctx, pr.ID, model.OwnerQuotation, other.ID,
		"other.pdf", "test/other.pdf", "application/pdf", 1024, user.ID, "")
	if err != nil {
		t.Fatalf("add other document: %v", err)
	}
	otherExt, err := repo.CreateExtraction(ctx, pr.ID, otherDoc.ID, &other.ID, "claude-opus-5", user.ID)
	if err != nil {
		t.Fatalf("create other extraction: %v", err)
	}
	if err := repo.FinishExtraction(ctx, otherExt.ID, repository.ExtractionSucceeded,
		[]byte(`{"vendor_name":"Exclusive Vendor","items":[]}`), 10, 10, ""); err != nil {
		t.Fatalf("finish other extraction: %v", err)
	}
	if err := repo.MarkExtractionApplied(ctx, otherExt.ID, other.ID, user.ID); err != nil {
		t.Fatalf("mark other applied: %v", err)
	}
	if !applied(final.ID) {
		t.Error("applying on another quotation cleared this quotation's applied stamp")
	}
	if !applied(otherExt.ID) {
		t.Error("other quotation's extraction not stamped")
	}
}
