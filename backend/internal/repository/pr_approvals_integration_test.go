package repository_test

import (
	"errors"
	"testing"

	"github.com/cs/purchasing-app/internal/model"
	"github.com/cs/purchasing-app/internal/repository"
)

// TestPRApprovals walks the approval feature end to end: a PR created with two
// approvers records per-approver decisions, supports reject + re-request, and
// honors the "keep at least one approver" guard.
func TestPRApprovals(t *testing.T) {
	repo, ctx := newTestRepo(t)

	requester, err := repo.UpsertUser(ctx, "appr-req-"+t.Name(), "appr-req@example.com", "Requester")
	if err != nil {
		t.Fatalf("upsert requester: %v", err)
	}
	a1, err := repo.UpsertUser(ctx, "appr-a1-"+t.Name(), "appr-a1@example.com", "Approver One")
	if err != nil {
		t.Fatalf("upsert approver1: %v", err)
	}
	a2, err := repo.UpsertUser(ctx, "appr-a2-"+t.Name(), "appr-a2@example.com", "Approver Two")
	if err != nil {
		t.Fatalf("upsert approver2: %v", err)
	}

	pr, err := repo.CreatePurchaseRequest(ctx, requester.ID, repository.PurchaseRequestInput{
		Title:       "Laptops with approval",
		ApproverIDs: []int64{a1.ID, a2.ID},
	})
	if err != nil {
		t.Fatalf("create PR: %v", err)
	}
	if pr.ApprovalsTotal != 2 || pr.ApprovalsApproved != 0 {
		t.Fatalf("new PR tally = %d/%d, want 0/2", pr.ApprovalsApproved, pr.ApprovalsTotal)
	}

	// Approver 1 approves; still one outstanding.
	if err := repo.RecordApprovalDecision(ctx, pr.ID, a1.ID, model.PRApprovalApproved, "ok"); err != nil {
		t.Fatalf("approve a1: %v", err)
	}

	// Approver 2 rejects, then the requester re-requests and a2 approves.
	if err := repo.RecordApprovalDecision(ctx, pr.ID, a2.ID, model.PRApprovalRejected, "needs cheaper option"); err != nil {
		t.Fatalf("reject a2: %v", err)
	}
	if err := repo.RequestApprovalAgain(ctx, pr.ID, a2.ID); err != nil {
		t.Fatalf("re-request a2: %v", err)
	}
	// Re-requesting a non-rejected approval is invalid.
	if err := repo.RequestApprovalAgain(ctx, pr.ID, a1.ID); !errors.Is(err, repository.ErrInvalidState) {
		t.Fatalf("re-request approved a1 = %v, want ErrInvalidState", err)
	}
	if err := repo.RecordApprovalDecision(ctx, pr.ID, a2.ID, model.PRApprovalApproved, ""); err != nil {
		t.Fatalf("approve a2: %v", err)
	}

	// Both approved: the tally reflects it.
	total, approved, err := repo.ApprovalTally(ctx, pr.ID)
	if err != nil || total != 2 || approved != 2 {
		t.Fatalf("tally = %d/%d err=%v, want 2/2", approved, total, err)
	}

	// Last-approver guard: removing one is fine, removing the last is refused.
	if err := repo.RemoveApprover(ctx, pr.ID, a1.ID); err != nil {
		t.Fatalf("remove a1: %v", err)
	}
	if err := repo.RemoveApprover(ctx, pr.ID, a2.ID); !errors.Is(err, repository.ErrInvalidState) {
		t.Fatalf("remove last approver = %v, want ErrInvalidState", err)
	}

	// A non-approver cannot record a decision.
	if err := repo.RecordApprovalDecision(ctx, pr.ID, requester.ID, model.PRApprovalApproved, ""); !errors.Is(err, repository.ErrInvalidState) {
		t.Fatalf("decision by non-approver = %v, want ErrInvalidState", err)
	}
}
