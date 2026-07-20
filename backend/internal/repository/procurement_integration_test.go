package repository_test

import (
	"testing"

	"github.com/cs/purchasing-app/internal/model"
	"github.com/cs/purchasing-app/internal/repository"
)

// TestProcurementLifecycle walks a purchase request through the full procurement
// workflow and asserts the auto-advance status transitions at each step.
func TestProcurementLifecycle(t *testing.T) {
	repo, ctx := newTestRepo(t)

	user, err := repo.UpsertUser(ctx, "proc-sub-"+t.Name(), "proc@example.com", "Proc Tester")
	if err != nil {
		t.Fatalf("upsert user: %v", err)
	}

	pr, err := repo.CreatePurchaseRequest(ctx, user.ID, repository.PurchaseRequestInput{Title: "Laptops"})
	if err != nil {
		t.Fatalf("create PR: %v", err)
	}
	if pr.Status != model.StatusSubmitted {
		t.Fatalf("new PR status = %q, want submitted", pr.Status)
	}

	vendor, err := repo.CreateVendor(ctx, repository.VendorInput{Name: "Acme"}, user.ID)
	if err != nil {
		t.Fatalf("create vendor: %v", err)
	}

	// Create quotation directly against the PR -> PR moves to under_review.
	validUntil := "2030-12-31"
	quo, err := repo.CreateQuotation(ctx, pr.ID, repository.QuotationInput{
		VendorID:    vendor.ID,
		TotalAmount: 1234.50,
		Currency:    "USD",
		ValidUntil:  &validUntil,
		Items: []repository.QuotationItem{
			{Description: "Laptop", Quantity: 3, UnitPrice: 411.50},
		},
	}, user.ID)
	if err != nil {
		t.Fatalf("create quotation: %v", err)
	}
	if got := reloadPRStatus(t, repo, pr.ID); got != model.StatusUnderReview {
		t.Fatalf("after quotation, PR status = %q, want under_review", got)
	}
	if quo.PurchaseRequestID != pr.ID {
		t.Fatalf("quotation PR id = %d, want %d", quo.PurchaseRequestID, pr.ID)
	}
	if quo.ValidUntil == nil || *quo.ValidUntil != validUntil {
		t.Fatalf("valid_until round-trip = %v, want %s", quo.ValidUntil, validUntil)
	}
	if len(quo.Items) != 1 || quo.Items[0].UnitPrice != 411.50 {
		t.Fatalf("quotation items not persisted: %+v", quo.Items)
	}

	// A fully-approved procurement recommendation gates selecting a quotation /
	// drafting a contract.
	if _, err := repo.CreateRecommendation(ctx, pr.ID, repository.RecommendationInput{VendorID: vendor.ID, Description: "go with Acme", RequiredTypes: []string{model.RecApprovalBudget}}, user.ID); err != nil {
		t.Fatalf("create recommendation: %v", err)
	}
	if err := repo.SelectQuotation(ctx, quo.ID); err != repository.ErrInvalidState {
		t.Fatalf("select before recommendation approved = %v, want ErrInvalidState", err)
	}
	if err := repo.SetRecApproval(ctx, pr.ID, model.RecApprovalBudget, user.ID); err != nil {
		t.Fatalf("approve budget card: %v", err)
	}

	// Select quotation -> PR moves to vendor_selected.
	if err := repo.SelectQuotation(ctx, quo.ID); err != nil {
		t.Fatalf("select quotation: %v", err)
	}
	if got := reloadPRStatus(t, repo, pr.ID); got != model.StatusVendorSelected {
		t.Fatalf("after select, PR status = %q, want vendor_selected", got)
	}

	// Create contract from the recommendation contract card (the only contract
	// creation path) -> PR moves to contract_prepared.
	contract, err := repo.CreateRecommendationContract(ctx, pr.ID, "Laptops contract", user.ID)
	if err != nil {
		t.Fatalf("create contract: %v", err)
	}
	if got := reloadPRStatus(t, repo, pr.ID); got != model.StatusContractPrepared {
		t.Fatalf("after contract, PR status = %q, want contract_prepared", got)
	}

	// Attach a signed PDF -> contract signed, PR order_signed. Signing is now
	// allowed straight from draft (contract review/approval was removed).
	doc, err := repo.AddOwnedDocument(ctx, pr.ID, model.OwnerContract, contract.ID, "signed.pdf", "p/signed.pdf", "application/pdf", 10, user.ID, "")
	if err != nil {
		t.Fatalf("add signed doc: %v", err)
	}
	if _, err := repo.SetContractSigned(ctx, contract.ID, doc.ID); err != nil {
		t.Fatalf("set contract signed: %v", err)
	}
	if got := reloadContractStatus(t, repo, contract.ID); got != model.ContractSigned {
		t.Fatalf("contract status = %q, want signed", got)
	}
	if got := reloadPRStatus(t, repo, pr.ID); got != model.StatusOrderSigned {
		t.Fatalf("after signing, PR status = %q, want order_signed", got)
	}

	// Deleting a quotation that a contract references must be refused.
	if _, err := repo.DeleteQuotation(ctx, quo.ID); err != repository.ErrHasChildren {
		t.Fatalf("DeleteQuotation with children err = %v, want ErrHasChildren", err)
	}
}

// TestQuotationDocuments asserts the primary quotation PDF is partitioned out of
// the other supporting documents, and that Set/Clear behave (replace returns the
// previous id; clear removes the FK + the document row).
func TestQuotationDocuments(t *testing.T) {
	repo, ctx := newTestRepo(t)

	user, err := repo.UpsertUser(ctx, "quo-doc-sub-"+t.Name(), "quodoc@example.com", "Quo Doc")
	if err != nil {
		t.Fatalf("upsert user: %v", err)
	}
	pr, err := repo.CreatePurchaseRequest(ctx, user.ID, repository.PurchaseRequestInput{Title: "Monitors"})
	if err != nil {
		t.Fatalf("create PR: %v", err)
	}
	vendor, err := repo.CreateVendor(ctx, repository.VendorInput{Name: "Screens Inc"}, user.ID)
	if err != nil {
		t.Fatalf("create vendor: %v", err)
	}
	quo, err := repo.CreateQuotation(ctx, pr.ID, repository.QuotationInput{VendorID: vendor.ID, Currency: "USD"}, user.ID)
	if err != nil {
		t.Fatalf("create quotation: %v", err)
	}

	addDoc := func(name, path string) int64 {
		d, err := repo.AddOwnedDocument(ctx, pr.ID, model.OwnerQuotation, quo.ID, name, path, "application/pdf", 10, user.ID, "")
		if err != nil {
			t.Fatalf("add doc %s: %v", name, err)
		}
		return d.ID
	}
	primary := addDoc("quotation.pdf", "p/quotation.pdf")
	other := addDoc("spec.pdf", "p/spec.pdf")

	// Before designating a primary, both are "other" documents.
	got, err := repo.GetQuotation(ctx, quo.ID)
	if err != nil {
		t.Fatalf("get quotation: %v", err)
	}
	if got.QuotationDocument != nil || len(got.Documents) != 2 {
		t.Fatalf("before set: primary=%v documents=%d, want nil primary + 2 others", got.QuotationDocument, len(got.Documents))
	}

	// Designate the primary; it is pulled out of Documents.
	if prev, err := repo.SetQuotationDocument(ctx, quo.ID, primary); err != nil || prev != nil {
		t.Fatalf("set primary = (%v, %v), want (nil, nil)", prev, err)
	}
	got, _ = repo.GetQuotation(ctx, quo.ID)
	if got.QuotationDocument == nil || got.QuotationDocument.ID != primary {
		t.Fatalf("primary doc = %+v, want id %d", got.QuotationDocument, primary)
	}
	if len(got.Documents) != 1 || got.Documents[0].ID != other {
		t.Fatalf("other documents = %+v, want [%d]", got.Documents, other)
	}

	// Replacing the primary returns the previously attached id so the caller can
	// clean up the old file/row (the handler does this — mirror it here).
	replacement := addDoc("quotation-v2.pdf", "p/quotation-v2.pdf")
	prev, err := repo.SetQuotationDocument(ctx, quo.ID, replacement)
	if err != nil || prev == nil || *prev != primary {
		t.Fatalf("replace primary = (%v, %v), want (%d, nil)", prev, err, primary)
	}
	if err := repo.DeleteOwnedDocument(ctx, model.OwnerQuotation, quo.ID, *prev); err != nil {
		t.Fatalf("delete replaced primary: %v", err)
	}

	// Clearing removes the FK and deletes the document row.
	if _, err := repo.ClearQuotationDocument(ctx, quo.ID); err != nil {
		t.Fatalf("clear primary: %v", err)
	}
	got, _ = repo.GetQuotation(ctx, quo.ID)
	if got.QuotationDocument != nil || got.QuotationDocumentID != nil {
		t.Fatalf("after clear: primary=%+v id=%v, want nil", got.QuotationDocument, got.QuotationDocumentID)
	}
	if len(got.Documents) != 1 || got.Documents[0].ID != other {
		t.Fatalf("after clear, other documents = %+v, want [%d]", got.Documents, other)
	}
	// Clearing again (nothing set) is refused.
	if _, err := repo.ClearQuotationDocument(ctx, quo.ID); err != repository.ErrInvalidState {
		t.Fatalf("clear with no primary = %v, want ErrInvalidState", err)
	}
}

func reloadPRStatus(t *testing.T, repo *repository.Repository, id int64) string {
	t.Helper()
	pr, err := repo.GetPurchaseRequest(t.Context(), id)
	if err != nil {
		t.Fatalf("reload PR: %v", err)
	}
	return pr.Status
}

func reloadContractStatus(t *testing.T, repo *repository.Repository, id int64) string {
	t.Helper()
	c, err := repo.GetContract(t.Context(), id)
	if err != nil {
		t.Fatalf("reload contract: %v", err)
	}
	return c.Status
}
