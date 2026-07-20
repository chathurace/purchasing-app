package repository_test

import (
	"testing"

	"github.com/cs/purchasing-app/internal/model"
	"github.com/cs/purchasing-app/internal/repository"
)

// TestEventLog exercises the append-only process/audit event writers: a valid
// action round-trips (with the DB-assigned created_at and the actor snapshot),
// and an action outside the fixed set is rejected rather than stored.
func TestEventLog(t *testing.T) {
	repo, ctx := newTestRepo(t)

	user, err := repo.UpsertUser(ctx, "evt-sub-"+t.Name(), "evt@example.com", "Event Tester")
	if err != nil {
		t.Fatalf("upsert user: %v", err)
	}
	pr, err := repo.CreatePurchaseRequest(ctx, user.ID, repository.PurchaseRequestInput{Title: "Monitors"})
	if err != nil {
		t.Fatalf("create PR: %v", err)
	}

	// Process event: a valid action is accepted.
	if err := repo.AddProcessEvent(ctx, pr.ID, model.ProcessSubmitPR, "", user.ID, user.Email); err != nil {
		t.Fatalf("add process event: %v", err)
	}
	if err := repo.AddProcessEvent(ctx, pr.ID, model.ProcessPRApproval, model.QualifierApprove, user.ID, user.Email); err != nil {
		t.Fatalf("add process event (approval): %v", err)
	}

	// Unknown action is rejected — the fixed-set guarantee.
	if err := repo.AddProcessEvent(ctx, pr.ID, "not_a_real_action", "", user.ID, user.Email); err == nil {
		t.Fatal("expected AddProcessEvent to reject an unknown action")
	}

	events, err := repo.ListProcessEvents(ctx, pr.ID)
	if err != nil {
		t.Fatalf("list process events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("process events = %d, want 2 (unknown action must not be stored)", len(events))
	}
	first := events[0]
	if first.Action != model.ProcessSubmitPR || first.Qualifier != "" {
		t.Fatalf("first event = %q/%q, want submit_pr/''", first.Action, first.Qualifier)
	}
	if first.ActorID == nil || *first.ActorID != user.ID {
		t.Fatalf("actor_id not recorded: %+v", first.ActorID)
	}
	if first.ActorEmail != user.Email {
		t.Fatalf("actor_email = %q, want %q", first.ActorEmail, user.Email)
	}
	if first.CreatedAt.IsZero() {
		t.Fatal("created_at was not populated by the DB default")
	}
	if events[1].Action != model.ProcessPRApproval || events[1].Qualifier != model.QualifierApprove {
		t.Fatalf("second event = %q/%q, want pr_approval/approve", events[1].Action, events[1].Qualifier)
	}

	// Audit event: valid action accepted, entity id optional, unknown rejected.
	vendorID := int64(4242)
	if err := repo.AddAuditEvent(ctx, model.AuditCreateVendor, "", model.EntityVendor, &vendorID, "Acme", user.ID, user.Email); err != nil {
		t.Fatalf("add audit event: %v", err)
	}
	if err := repo.AddAuditEvent(ctx, model.AuditConnectStorage, "", model.EntityStorage, nil, "acct@example.com", user.ID, user.Email); err != nil {
		t.Fatalf("add audit event (nil entity id): %v", err)
	}
	if err := repo.AddAuditEvent(ctx, "not_a_real_action", "", model.EntityVendor, &vendorID, "", user.ID, user.Email); err == nil {
		t.Fatal("expected AddAuditEvent to reject an unknown action")
	}
}
