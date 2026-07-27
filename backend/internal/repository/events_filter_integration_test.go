package repository_test

import (
	"testing"

	"github.com/cs/purchasing-app/internal/model"
	"github.com/cs/purchasing-app/internal/repository"
)

// TestFilterEvents exercises the admin events-view reads (FilterProcessEvents /
// FilterAuditEvents) across the actor / action / PR-id filters. It creates its
// own PR + events and cleans them up (process_events cascade with the PR; audit
// rows are deleted explicitly).
func TestFilterEvents(t *testing.T) {
	repo, ctx := newTestRepo(t)

	actor, err := repo.UpsertUser(ctx, "evt-sub-"+t.Name(), "evt-actor@example.com", "Evt Actor")
	if err != nil {
		t.Fatalf("upsert user: %v", err)
	}
	pr, err := repo.CreatePurchaseRequest(ctx, actor.ID, repository.PurchaseRequestInput{Title: "Evt PR"})
	if err != nil {
		t.Fatalf("create PR: %v", err)
	}
	// Deleting the PR cascades its process_events.
	t.Cleanup(func() { _, _ = repo.Pool().Exec(ctx, `DELETE FROM purchase_requests WHERE id=$1`, pr.ID) })

	// Two process events on the PR.
	if err := repo.AddProcessEvent(ctx, pr.ID, model.ProcessSubmitPR, "", actor.ID, actor.Email); err != nil {
		t.Fatalf("add process submit: %v", err)
	}
	if err := repo.AddProcessEvent(ctx, pr.ID, model.ProcessPRApproval, model.QualifierApprove, actor.ID, actor.Email); err != nil {
		t.Fatalf("add process approval: %v", err)
	}
	// One audit event by the same actor.
	if err := repo.AddAuditEvent(ctx, model.AuditCreateVendor, "", model.EntityVendor, nil, "Acme Corp", actor.ID, actor.Email); err != nil {
		t.Fatalf("add audit: %v", err)
	}
	t.Cleanup(func() {
		_, _ = repo.Pool().Exec(ctx, `DELETE FROM audit_events WHERE actor_email = $1`, actor.Email)
	})

	// PR-id filter → both process events, newest-first.
	proc, err := repo.FilterProcessEvents(ctx, repository.EventFilter{PRID: &pr.ID})
	if err != nil {
		t.Fatalf("filter process: %v", err)
	}
	if len(proc) != 2 {
		t.Fatalf("want 2 process events, got %d", len(proc))
	}
	if proc[0].Action != model.ProcessPRApproval {
		t.Errorf("want newest-first (approval), got %q", proc[0].Action)
	}
	if proc[0].ActorName != "Evt Actor" {
		t.Errorf("want joined actor name, got %q", proc[0].ActorName)
	}

	// Action filter narrows to one.
	proc, err = repo.FilterProcessEvents(ctx, repository.EventFilter{PRID: &pr.ID, Action: model.ProcessSubmitPR})
	if err != nil {
		t.Fatalf("filter process by action: %v", err)
	}
	if len(proc) != 1 || proc[0].Action != model.ProcessSubmitPR {
		t.Fatalf("want 1 submit_pr, got %+v", proc)
	}

	// Actor substring (case-insensitive, matches email snapshot).
	proc, err = repo.FilterProcessEvents(ctx, repository.EventFilter{PRID: &pr.ID, ActorTerm: "EVT-ACTOR"})
	if err != nil {
		t.Fatalf("filter process by actor: %v", err)
	}
	if len(proc) != 2 {
		t.Errorf("actor substring should match both, got %d", len(proc))
	}

	// Date-range filter (inclusive ::date bounds) — a wide window keeps both.
	proc, err = repo.FilterProcessEvents(ctx, repository.EventFilter{PRID: &pr.ID, From: "2000-01-01", To: "2999-12-31"})
	if err != nil {
		t.Fatalf("filter process by date: %v", err)
	}
	if len(proc) != 2 {
		t.Errorf("wide date window should keep both, got %d", len(proc))
	}
	// A window entirely in the past excludes them.
	proc, err = repo.FilterProcessEvents(ctx, repository.EventFilter{PRID: &pr.ID, From: "2000-01-01", To: "2000-01-02"})
	if err != nil {
		t.Fatalf("filter process by past date: %v", err)
	}
	if len(proc) != 0 {
		t.Errorf("past date window should exclude all, got %d", len(proc))
	}

	// Audit read + action/actor filters.
	audit, err := repo.FilterAuditEvents(ctx, repository.EventFilter{ActorTerm: "evt-actor@example.com"})
	if err != nil {
		t.Fatalf("filter audit: %v", err)
	}
	if len(audit) != 1 || audit[0].Action != model.AuditCreateVendor || audit[0].Detail != "Acme Corp" {
		t.Fatalf("want 1 create_vendor(Acme Corp), got %+v", audit)
	}
	if audit[0].ActorName != "Evt Actor" {
		t.Errorf("want joined actor name on audit, got %q", audit[0].ActorName)
	}
}
