package handler

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/cs/purchasing-app/internal/model"
	"github.com/cs/purchasing-app/internal/repository"
)

// TestUpdateQuotationMarksExtractionApplied covers the bug behind a card showing
// "not applied" after the user had just applied a PDF's extracted details.
//
// Create had its own adoption path that stamped applied_at, but Update — the
// post-create "Apply" on a card's PDF slot — silently ignored extraction_id, so
// anything applied from a card stayed marked "not applied" forever.
func TestUpdateQuotationMarksExtractionApplied(t *testing.T) {
	repo, ctx := newHandlerTestRepo(t)

	user, err := repo.UpsertUser(ctx, "h-xapply-"+t.Name(), "h-xapply-"+t.Name()+"@example.com", "Procurement")
	if err != nil {
		t.Fatalf("upsert user: %v", err)
	}
	if err := repo.EnsureUserHasRole(ctx, user.ID, model.RoleProcurement); err != nil {
		t.Fatalf("grant procurement role: %v", err)
	}
	pr, err := repo.CreatePurchaseRequest(ctx, user.ID, repository.PurchaseRequestInput{Title: "Apply stamp"})
	if err != nil {
		t.Fatalf("create PR: %v", err)
	}
	vendor, err := repo.CreateVendor(ctx, repository.VendorInput{Name: "Stamp Vendor Ltd"}, user.ID)
	if err != nil {
		t.Fatalf("create vendor: %v", err)
	}
	t.Cleanup(func() {
		if _, err := repo.Pool().Exec(ctx, `DELETE FROM purchase_requests WHERE id=$1`, pr.ID); err != nil {
			t.Errorf("cleanup purchase request: %v", err)
		}
		if _, err := repo.Pool().Exec(ctx, `DELETE FROM vendors WHERE id=$1`, vendor.ID); err != nil {
			t.Errorf("cleanup vendor: %v", err)
		}
		if _, err := repo.Pool().Exec(ctx, `DELETE FROM users WHERE id=$1`, user.ID); err != nil {
			t.Errorf("cleanup user: %v", err)
		}
	})

	quo, err := repo.CreateQuotation(ctx, pr.ID, repository.QuotationInput{
		VendorID: vendor.ID, TotalAmount: 0, Currency: "USD",
	}, user.ID)
	if err != nil {
		t.Fatalf("create quotation: %v", err)
	}

	// A read of the quotation's PDF, succeeded but not yet applied.
	doc, err := repo.AddOwnedDocument(ctx, pr.ID, model.OwnerQuotation, quo.ID,
		"quote.pdf", "test/quote.pdf", "application/pdf", 2048, user.ID, "")
	if err != nil {
		t.Fatalf("add document: %v", err)
	}
	ext, err := repo.CreateExtraction(ctx, pr.ID, doc.ID, &quo.ID, "claude-opus-5", user.ID)
	if err != nil {
		t.Fatalf("create extraction: %v", err)
	}
	if err := repo.FinishExtraction(ctx, ext.ID, repository.ExtractionSucceeded,
		[]byte(`{"vendor_name":"Stamp Vendor","currency":"USD","total_amount":1200,"items":[]}`),
		100, 50, ""); err != nil {
		t.Fatalf("finish extraction: %v", err)
	}

	h := &QuotationsHandler{Repo: repo}

	// Apply the reviewed values, exactly as the card's Apply button does.
	body := `{"vendor_id":` + id64(vendor.ID) + `,"total_amount":1200,"currency":"USD",` +
		`"valid_until":null,"notes":"","items":[],"extraction_id":` + id64(ext.ID) + `}`
	rec := httptest.NewRecorder()
	h.Update(rec, withID(authed(user, []string{model.RoleProcurement}, body), quo.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("update = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}

	reloaded, err := repo.GetExtraction(ctx, ext.ID)
	if err != nil {
		t.Fatalf("reload extraction: %v", err)
	}
	if reloaded.AppliedAt == nil {
		t.Error("applied_at not stamped after Apply; the card would keep showing \"not applied\"")
	}
	if reloaded.QuotationID == nil || *reloaded.QuotationID != quo.ID {
		t.Errorf("quotation_id = %v, want %d", reloaded.QuotationID, quo.ID)
	}

	// An extraction from another PR must not be applicable to this quotation.
	otherPR, err := repo.CreatePurchaseRequest(ctx, user.ID, repository.PurchaseRequestInput{Title: "Other PR"})
	if err != nil {
		t.Fatalf("create other PR: %v", err)
	}
	t.Cleanup(func() {
		if _, err := repo.Pool().Exec(ctx, `DELETE FROM purchase_requests WHERE id=$1`, otherPR.ID); err != nil {
			t.Errorf("cleanup other purchase request: %v", err)
		}
	})
	otherDoc, err := repo.AddOwnedDocument(ctx, otherPR.ID, model.OwnerQuotationExtraction, 0,
		"foreign.pdf", "test/foreign.pdf", "application/pdf", 1024, user.ID, "")
	if err != nil {
		t.Fatalf("add foreign document: %v", err)
	}
	foreign, err := repo.CreateExtraction(ctx, otherPR.ID, otherDoc.ID, nil, "claude-opus-5", user.ID)
	if err != nil {
		t.Fatalf("create foreign extraction: %v", err)
	}
	if err := repo.FinishExtraction(ctx, foreign.ID, repository.ExtractionSucceeded,
		[]byte(`{"vendor_name":"Elsewhere","items":[]}`), 10, 10, ""); err != nil {
		t.Fatalf("finish foreign extraction: %v", err)
	}

	body = `{"vendor_id":` + id64(vendor.ID) + `,"total_amount":1200,"currency":"USD",` +
		`"valid_until":null,"notes":"","items":[],"extraction_id":` + id64(foreign.ID) + `}`
	rec = httptest.NewRecorder()
	h.Update(rec, withID(authed(user, []string{model.RoleProcurement}, body), quo.ID))
	// The update itself still succeeds — the stamp is best-effort, not a gate — but the
	// foreign extraction must not be claimed by this quotation.
	if rec.Code != http.StatusOK {
		t.Fatalf("update with a foreign extraction id = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	reloadedForeign, err := repo.GetExtraction(ctx, foreign.ID)
	if err != nil {
		t.Fatalf("reload foreign extraction: %v", err)
	}
	if reloadedForeign.AppliedAt != nil {
		t.Error("an extraction from another PR was stamped applied against this quotation")
	}
	if reloadedForeign.QuotationID != nil {
		t.Errorf("foreign extraction quotation_id = %v, want nil", *reloadedForeign.QuotationID)
	}
}

// id64 renders an id for inlining into a JSON test body.
func id64(n int64) string { return strconv.FormatInt(n, 10) }
