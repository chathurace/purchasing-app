package repository_test

import (
	"testing"

	"github.com/cs/purchasing-app/internal/model"
	"github.com/cs/purchasing-app/internal/repository"
)

// TestProcurementLifecycle walks a purchase request through the full finance
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
	if _, err := repo.CreateRecommendation(ctx, pr.ID, vendor.ID, "go with Acme", []string{model.RecApprovalBudget}, user.ID); err != nil {
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

	// Create contract -> PR moves to contract_prepared.
	contract, err := repo.CreateContractFromQuotation(ctx, quo.ID, repository.ContractInput{
		Title: "Laptops contract", TotalAmount: 1234.50, Currency: "USD",
	}, user.ID)
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
