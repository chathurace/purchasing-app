package repository_test

import (
	"context"
	"testing"

	"github.com/cs/purchasing-app/internal/repository"
)

// TestHomeDashboardCounts exercises the home-page aggregation queries end-to-end
// against the dev DB: the requester counts are asserted precisely (scoped to a
// freshly created user), and the other role queries are checked for validity.
func TestHomeDashboardCounts(t *testing.T) {
	repo, ctx := newTestRepo(t)

	requester, err := repo.UpsertUser(ctx, "home-sub-"+t.Name(), "home-requester@example.com", "Home Requester")
	if err != nil {
		t.Fatalf("upsert user: %v", err)
	}

	// Two requests: one left submitted, one advanced to order_signed (counts as
	// "completed" on the staff card).
	var prIDs []int64
	for i := 0; i < 2; i++ {
		pr, err := repo.CreatePurchaseRequest(ctx, requester.ID, repository.PurchaseRequestInput{
			Title: "Home test PR",
		})
		if err != nil {
			t.Fatalf("create pr: %v", err)
		}
		prIDs = append(prIDs, pr.ID)
	}
	t.Cleanup(func() {
		for _, id := range prIDs {
			_, _ = repo.Pool().Exec(context.Background(), `DELETE FROM purchase_requests WHERE id=$1`, id)
		}
		_, _ = repo.Pool().Exec(context.Background(), `DELETE FROM users WHERE id=$1`, requester.ID)
	})
	if _, err := repo.Pool().Exec(ctx, `UPDATE purchase_requests SET status='order_signed' WHERE id=$1`, prIDs[1]); err != nil {
		t.Fatalf("advance status: %v", err)
	}

	total, completed, err := repo.CountMyRequests(ctx, requester.ID)
	if err != nil {
		t.Fatalf("CountMyRequests: %v", err)
	}
	if total != 2 || completed != 1 {
		t.Errorf("CountMyRequests = (%d,%d), want (2,1)", total, completed)
	}

	// A process event on one of the PRs surfaces in the requester activity feed.
	if err := repo.AddProcessEvent(ctx, prIDs[0], "submit_pr", "", requester.ID, requester.Email); err != nil {
		t.Fatalf("add event: %v", err)
	}
	act, err := repo.RecentActivityForRequester(ctx, requester.ID, 8)
	if err != nil {
		t.Fatalf("RecentActivityForRequester: %v", err)
	}
	found := false
	for _, a := range act {
		if a.PurchaseRequestID == prIDs[0] && a.Action == "submit_pr" {
			found = true
			if a.Title == "" {
				t.Errorf("activity row missing PR title")
			}
		}
	}
	if !found {
		t.Errorf("expected submit_pr activity for PR %d, got %+v", prIDs[0], act)
	}

	// The approver and procurement queries must at least execute cleanly.
	if _, _, err := repo.CountApprovals(ctx, requester.ID, requester.Email, nil); err != nil {
		t.Fatalf("CountApprovals: %v", err)
	}
	pending, awaiting, done, err := repo.CountProcurementRequests(ctx, true)
	if err != nil {
		t.Fatalf("CountProcurementRequests: %v", err)
	}
	// Admin sees all non-terminal PRs; our submitted PR is one of the pending set
	// and the order_signed PR (no invoices) is awaiting delivery.
	if pending < 1 {
		t.Errorf("expected >=1 pending procurement request, got %d", pending)
	}
	if awaiting < 1 {
		t.Errorf("expected >=1 awaiting-delivery request, got %d", awaiting)
	}
	_ = done
	if _, err := repo.RecentActivityForProcurement(ctx, true, 8); err != nil {
		t.Fatalf("RecentActivityForProcurement: %v", err)
	}
	if _, err := repo.RecentActivityForApprover(ctx, requester.ID, requester.Email, nil, 8); err != nil {
		t.Fatalf("RecentActivityForApprover: %v", err)
	}
}
