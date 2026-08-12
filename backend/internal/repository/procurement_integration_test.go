package repository_test

import (
	"testing"

	"github.com/cs/purchasing-app/internal/model"
	"github.com/cs/purchasing-app/internal/repository"
	"github.com/jackc/pgx/v5"
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
	// Budget approval is a serial chain: approve the base step (seeded on create).
	if _, err := repo.SetBudgetStepDecision(ctx, pr.ID, baseBudgetStepID(t, repo, pr.ID), "approve", user.ID); err != nil {
		t.Fatalf("approve budget base step: %v", err)
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

// TestQuotationDocuments asserts the two primary quotation PDFs (initial/final)
// are partitioned out of the other supporting documents, that the slots are
// independent, and that Set/Clear behave (replace returns the previous id; clear
// removes the FK + the document row).
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
	initial := addDoc("quotation.pdf", "p/quotation.pdf")
	final := addDoc("quotation-final.pdf", "p/quotation-final.pdf")
	other := addDoc("spec.pdf", "p/spec.pdf")

	// Before designating the primaries, all three are "other" documents.
	got, err := repo.GetQuotation(ctx, quo.ID)
	if err != nil {
		t.Fatalf("get quotation: %v", err)
	}
	if got.InitialQuotationDocument != nil || got.FinalQuotationDocument != nil || len(got.Documents) != 3 {
		t.Fatalf("before set: initial=%v final=%v documents=%d, want nil primaries + 3 others",
			got.InitialQuotationDocument, got.FinalQuotationDocument, len(got.Documents))
	}

	// Designate the initial PDF; it is pulled out of Documents. The final slot is
	// independent and stays empty (a quotation is usable without it).
	if prev, err := repo.SetQuotationDocument(ctx, quo.ID, repository.QuotationDocInitial, initial); err != nil || prev != nil {
		t.Fatalf("set initial = (%v, %v), want (nil, nil)", prev, err)
	}
	got, _ = repo.GetQuotation(ctx, quo.ID)
	if got.InitialQuotationDocument == nil || got.InitialQuotationDocument.ID != initial {
		t.Fatalf("initial doc = %+v, want id %d", got.InitialQuotationDocument, initial)
	}
	if got.FinalQuotationDocument != nil {
		t.Fatalf("final doc = %+v, want nil", got.FinalQuotationDocument)
	}
	if len(got.Documents) != 2 {
		t.Fatalf("other documents = %+v, want 2", got.Documents)
	}

	// Designate the final PDF; both primaries now sit outside Documents.
	if prev, err := repo.SetQuotationDocument(ctx, quo.ID, repository.QuotationDocFinal, final); err != nil || prev != nil {
		t.Fatalf("set final = (%v, %v), want (nil, nil)", prev, err)
	}
	got, _ = repo.GetQuotation(ctx, quo.ID)
	if got.FinalQuotationDocument == nil || got.FinalQuotationDocument.ID != final {
		t.Fatalf("final doc = %+v, want id %d", got.FinalQuotationDocument, final)
	}
	if len(got.Documents) != 1 || got.Documents[0].ID != other {
		t.Fatalf("other documents = %+v, want [%d]", got.Documents, other)
	}

	// The summaries used by the PR-page cards carry both primaries.
	summaries, err := repo.ListQuotations(ctx, &pr.ID)
	if err != nil || len(summaries) != 1 {
		t.Fatalf("list quotations = (%d rows, %v), want 1 row", len(summaries), err)
	}
	if s := summaries[0]; s.InitialQuotationDocument == nil || s.InitialQuotationDocument.ID != initial ||
		s.FinalQuotationDocument == nil || s.FinalQuotationDocument.ID != final {
		t.Fatalf("summary primaries = (%+v, %+v), want ids (%d, %d)",
			summaries[0].InitialQuotationDocument, summaries[0].FinalQuotationDocument, initial, final)
	}

	// Replacing a primary returns the previously attached id so the caller can
	// clean up the old file/row (the handler does this — mirror it here).
	replacement := addDoc("quotation-v2.pdf", "p/quotation-v2.pdf")
	prev, err := repo.SetQuotationDocument(ctx, quo.ID, repository.QuotationDocInitial, replacement)
	if err != nil || prev == nil || *prev != initial {
		t.Fatalf("replace initial = (%v, %v), want (%d, nil)", prev, err, initial)
	}
	if err := repo.DeleteOwnedDocument(ctx, model.OwnerQuotation, quo.ID, *prev); err != nil {
		t.Fatalf("delete replaced initial: %v", err)
	}

	// Clearing removes the FK and deletes the document row — per slot.
	if _, err := repo.ClearQuotationDocument(ctx, quo.ID, repository.QuotationDocInitial); err != nil {
		t.Fatalf("clear initial: %v", err)
	}
	got, _ = repo.GetQuotation(ctx, quo.ID)
	if got.InitialQuotationDocument != nil || got.InitialQuotationDocumentID != nil {
		t.Fatalf("after clear: initial=%+v id=%v, want nil", got.InitialQuotationDocument, got.InitialQuotationDocumentID)
	}
	if got.FinalQuotationDocument == nil || got.FinalQuotationDocument.ID != final {
		t.Fatalf("clearing the initial slot disturbed the final one: %+v", got.FinalQuotationDocument)
	}
	if len(got.Documents) != 1 || got.Documents[0].ID != other {
		t.Fatalf("after clear, other documents = %+v, want [%d]", got.Documents, other)
	}
	// Clearing again (nothing set) is refused.
	if _, err := repo.ClearQuotationDocument(ctx, quo.ID, repository.QuotationDocInitial); err != repository.ErrInvalidState {
		t.Fatalf("clear with no initial = %v, want ErrInvalidState", err)
	}
	if _, err := repo.ClearQuotationDocument(ctx, quo.ID, repository.QuotationDocFinal); err != nil {
		t.Fatalf("clear final: %v", err)
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

// TestRecApprovalCardAddRemove exercises requesting and removing individual
// approval cards on a recommendation without disturbing the others.
func TestRecApprovalCardAddRemove(t *testing.T) {
	repo, ctx := newTestRepo(t)

	user, err := repo.UpsertUser(ctx, "card-sub-"+t.Name(), "card-proc@example.com", "Card Proc")
	if err != nil {
		t.Fatalf("upsert user: %v", err)
	}
	pr, err := repo.CreatePurchaseRequest(ctx, user.ID, repository.PurchaseRequestInput{Title: "Cards"})
	if err != nil {
		t.Fatalf("create PR: %v", err)
	}
	vendor, err := repo.CreateVendor(ctx, repository.VendorInput{Name: "CardVendor " + t.Name()}, user.ID)
	if err != nil {
		t.Fatalf("create vendor: %v", err)
	}
	if _, err := repo.CreateQuotation(ctx, pr.ID, repository.QuotationInput{VendorID: vendor.ID, TotalAmount: 10, Currency: "USD"}, user.ID); err != nil {
		t.Fatalf("create quotation: %v", err)
	}
	if _, err := repo.CreateRecommendation(ctx, pr.ID, repository.RecommendationInput{
		VendorID: vendor.ID, Description: "cards", RequiredTypes: []string{model.RecApprovalBudget},
	}, user.ID); err != nil {
		t.Fatalf("create recommendation: %v", err)
	}

	// Request the legal card -> now two cards; budget seeded a base step, legal none.
	if err := repo.AddRecApprovalCard(ctx, pr.ID, model.RecApprovalLegal); err != nil {
		t.Fatalf("add legal card: %v", err)
	}
	rec, err := repo.GetRecommendation(ctx, pr.ID)
	if err != nil {
		t.Fatalf("get recommendation: %v", err)
	}
	if len(rec.Approvals) != 2 {
		t.Fatalf("want 2 cards, got %d", len(rec.Approvals))
	}
	// Requesting an already-present card is refused.
	if err := repo.AddRecApprovalCard(ctx, pr.ID, model.RecApprovalLegal); err != repository.ErrInvalidState {
		t.Fatalf("re-add legal = %v, want ErrInvalidState", err)
	}

	// Remove the legal card again.
	if _, err := repo.RemoveRecApprovalCard(ctx, pr.ID, model.RecApprovalLegal); err != nil {
		t.Fatalf("remove legal card: %v", err)
	}
	// Removing a card that isn't required -> not found.
	if _, err := repo.RemoveRecApprovalCard(ctx, pr.ID, model.RecApprovalSecurity); err != pgx.ErrNoRows {
		t.Fatalf("remove absent security = %v, want ErrNoRows", err)
	}
	// The last remaining card cannot be removed.
	if _, err := repo.RemoveRecApprovalCard(ctx, pr.ID, model.RecApprovalBudget); err != repository.ErrInvalidState {
		t.Fatalf("remove last (budget) = %v, want ErrInvalidState", err)
	}
	// Budget base step survived the legal add/remove churn.
	if bc := budgetCard(t, repo, pr.ID); len(bc.BudgetSteps) != 1 {
		t.Fatalf("budget base step count = %d, want 1", len(bc.BudgetSteps))
	}

	if _, err := repo.DeleteRecommendation(ctx, pr.ID); err != nil {
		t.Fatalf("cleanup recommendation: %v", err)
	}
}

// budgetCard returns the budget approval card of a PR's recommendation.
func budgetCard(t *testing.T, repo *repository.Repository, prID int64) *repository.RecApproval {
	t.Helper()
	rec, err := repo.GetRecommendation(t.Context(), prID)
	if err != nil || rec == nil {
		t.Fatalf("get recommendation: %v", err)
	}
	for i := range rec.Approvals {
		if rec.Approvals[i].ApprovalType == model.RecApprovalBudget {
			return &rec.Approvals[i]
		}
	}
	t.Fatalf("no budget card on recommendation")
	return nil
}

// baseBudgetStepID returns the id of the recommendation's base budget step.
func baseBudgetStepID(t *testing.T, repo *repository.Repository, prID int64) int64 {
	t.Helper()
	card := budgetCard(t, repo, prID)
	for _, s := range card.BudgetSteps {
		if s.IsBase {
			return s.ID
		}
	}
	t.Fatalf("no base budget step")
	return 0
}

// TestBudgetApprovalChain exercises the serial budget approval chain end-to-end:
// step seeding, serial ordering, the projected budget card, and the lock rule.
func TestBudgetApprovalChain(t *testing.T) {
	repo, ctx := newTestRepo(t)

	user, err := repo.UpsertUser(ctx, "chain-sub-"+t.Name(), "chain-proc@example.com", "Chain Proc")
	if err != nil {
		t.Fatalf("upsert user: %v", err)
	}
	pr, err := repo.CreatePurchaseRequest(ctx, user.ID, repository.PurchaseRequestInput{Title: "Chain"})
	if err != nil {
		t.Fatalf("create PR: %v", err)
	}
	vendor, err := repo.CreateVendor(ctx, repository.VendorInput{Name: "ChainVendor " + t.Name()}, user.ID)
	if err != nil {
		t.Fatalf("create vendor: %v", err)
	}
	quo, err := repo.CreateQuotation(ctx, pr.ID, repository.QuotationInput{VendorID: vendor.ID, TotalAmount: 100, Currency: "USD"}, user.ID)
	if err != nil {
		t.Fatalf("create quotation: %v", err)
	}
	if _, err := repo.CreateRecommendation(ctx, pr.ID, repository.RecommendationInput{
		VendorID: vendor.ID, Description: "chain", RequiredTypes: []string{model.RecApprovalBudget},
	}, user.ID); err != nil {
		t.Fatalf("create recommendation: %v", err)
	}

	// A base step is seeded on create; the chain is not yet approved.
	card := budgetCard(t, repo, pr.ID)
	if len(card.BudgetSteps) != 1 || !card.BudgetSteps[0].IsBase {
		t.Fatalf("want 1 base step, got %+v", card.BudgetSteps)
	}
	if ok, _ := repo.RecommendationFullyApproved(ctx, pr.ID); ok {
		t.Fatal("fully approved with a pending base step")
	}
	baseID := card.BudgetSteps[0].ID

	// Add two named steps.
	s2, err := repo.AddBudgetStep(ctx, pr.ID, "Finance", "finance@example.com")
	if err != nil {
		t.Fatalf("add step 2: %v", err)
	}
	s3, err := repo.AddBudgetStep(ctx, pr.ID, "CFO", "cfo@example.com")
	if err != nil {
		t.Fatalf("add step 3: %v", err)
	}
	if s2.Position != 2 || s3.Position != 3 {
		t.Fatalf("positions = %d,%d want 2,3", s2.Position, s3.Position)
	}

	// A named step approver is recognised as an approver of the PR (the budget-step
	// branch of approvablePredicate), matched case-insensitively by email; an
	// unrelated address is not.
	if ok, err := repo.IsApproverForPR(ctx, pr.ID, 0, "FINANCE@example.com", nil); err != nil || !ok {
		t.Fatalf("named step approver IsApproverForPR = %v (err %v), want true", ok, err)
	}
	if ok, _ := repo.IsApproverForPR(ctx, pr.ID, 0, "stranger@example.com", nil); ok {
		t.Fatal("unrelated email should not be an approver")
	}

	// Serial gate: step 2 cannot be decided before the base step is approved.
	if _, err := repo.SetBudgetStepDecision(ctx, pr.ID, s2.ID, "approve", user.ID); err != repository.ErrInvalidState {
		t.Fatalf("approve step 2 before base = %v, want ErrInvalidState", err)
	}

	// Approve base -> still not fully approved (steps 2,3 pending).
	if _, err := repo.SetBudgetStepDecision(ctx, pr.ID, baseID, "approve", user.ID); err != nil {
		t.Fatalf("approve base: %v", err)
	}
	if ok, _ := repo.RecommendationFullyApproved(ctx, pr.ID); ok {
		t.Fatal("fully approved with pending later steps")
	}

	// Approve step 2, then step 3 -> the projected budget card becomes approved.
	if _, err := repo.SetBudgetStepDecision(ctx, pr.ID, s2.ID, "approve", user.ID); err != nil {
		t.Fatalf("approve step 2: %v", err)
	}
	if _, err := repo.SetBudgetStepDecision(ctx, pr.ID, s3.ID, "approve", user.ID); err != nil {
		t.Fatalf("approve step 3: %v", err)
	}
	if ok, _ := repo.RecommendationFullyApproved(ctx, pr.ID); !ok {
		t.Fatal("not fully approved after every step approved")
	}
	if bc := budgetCard(t, repo, pr.ID); !bc.Approved {
		t.Fatal("budget card projection not approved")
	}

	// Lock rule: step 2 cannot be reversed now that step 3 has decided.
	if _, err := repo.SetBudgetStepDecision(ctx, pr.ID, s2.ID, "revert", user.ID); err != repository.ErrInvalidState {
		t.Fatalf("reverse locked step 2 = %v, want ErrInvalidState", err)
	}

	// Reject flow: revert step 3, then reject step 2 -> chain no longer approved and
	// the quotation cannot be selected.
	if _, err := repo.SetBudgetStepDecision(ctx, pr.ID, s3.ID, "revert", user.ID); err != nil {
		t.Fatalf("revert step 3: %v", err)
	}
	if _, err := repo.SetBudgetStepDecision(ctx, pr.ID, s2.ID, "reject", user.ID); err != nil {
		t.Fatalf("reject step 2: %v", err)
	}
	if ok, _ := repo.RecommendationFullyApproved(ctx, pr.ID); ok {
		t.Fatal("fully approved with a rejected step")
	}
	if err := repo.SelectQuotation(ctx, quo.ID); err != repository.ErrInvalidState {
		t.Fatalf("select with rejected step = %v, want ErrInvalidState", err)
	}

	// A pending additional step can be removed; a decided one cannot.
	if err := repo.DeleteBudgetStep(ctx, pr.ID, s3.ID); err != nil {
		t.Fatalf("delete pending step 3: %v", err)
	}
	if err := repo.DeleteBudgetStep(ctx, pr.ID, s2.ID); err != repository.ErrInvalidState {
		t.Fatalf("delete rejected step 2 = %v, want ErrInvalidState", err)
	}

	// Clean up the test data.
	if _, err := repo.DeleteRecommendation(ctx, pr.ID); err != nil {
		t.Fatalf("cleanup recommendation: %v", err)
	}
}
