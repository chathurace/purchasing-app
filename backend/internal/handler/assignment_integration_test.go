package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/cs/purchasing-app/internal/model"
	"github.com/cs/purchasing-app/internal/repository"
)

// TestProcurementAdminSelfAssign proves a procurement_admin who does NOT also hold
// the plain procurement role can still assign a (team-lead-approved) PR to
// themselves — the assignee pool includes procurement_admin/admin, not only the
// Procurement team.
func TestProcurementAdminSelfAssign(t *testing.T) {
	repo, ctx := newHandlerTestRepo(t)

	requester, err := repo.UpsertUser(ctx, "h-asg-req-"+t.Name(), "h-asg-req-"+t.Name()+"@example.com", "Requester")
	if err != nil {
		t.Fatalf("upsert requester: %v", err)
	}
	lead, err := repo.UpsertUser(ctx, "h-asg-lead-"+t.Name(), "h-asg-lead-"+t.Name()+"@example.com", "Lead")
	if err != nil {
		t.Fatalf("upsert lead: %v", err)
	}
	// A procurement_admin who holds ONLY procurement_admin (not plain procurement).
	padmin, err := repo.UpsertUser(ctx, "h-asg-padmin-"+t.Name(), "h-asg-padmin-"+t.Name()+"@example.com", "Proc Admin")
	if err != nil {
		t.Fatalf("upsert procurement_admin: %v", err)
	}
	if err := repo.EnsureUserHasRole(ctx, padmin.ID, model.RoleProcurementAdmin); err != nil {
		t.Fatalf("grant procurement_admin role: %v", err)
	}

	h := &PurchaseRequestsHandler{Repo: repo}

	rec := httptest.NewRecorder()
	h.Create(rec, authed(requester, []string{model.RoleStaff},
		`{"title":"Admin self-assign","team_lead_email":"`+lead.Email+`"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d, want 201 (body=%s)", rec.Code, rec.Body.String())
	}
	var created repository.PurchaseRequest
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created PR: %v", err)
	}
	t.Cleanup(func() {
		_, _ = repo.Pool().Exec(context.Background(), `DELETE FROM purchase_requests WHERE id=$1`, created.ID)
	})

	// Team lead approves so procurement can act.
	rec = httptest.NewRecorder()
	h.TeamLeadDecision(rec, withID(authed(lead, []string{model.RoleStaff}, `{"decision":"approve","notes":"ok"}`), created.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("team lead approve = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}

	// The procurement_admin assigns the PR to themselves.
	rec = httptest.NewRecorder()
	h.SetAssignee(rec, withID(authed(padmin, []string{model.RoleProcurementAdmin},
		`{"assignee_id":`+strconv.FormatInt(padmin.ID, 10)+`}`), created.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("procurement_admin self-assign = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var updated repository.PurchaseRequest
	if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode updated PR: %v", err)
	}
	if updated.AssigneeID == nil || *updated.AssigneeID != padmin.ID {
		t.Fatalf("assignee = %v, want %d", updated.AssigneeID, padmin.ID)
	}
}

// TestSetPriority covers the PR priority triage control on the assignment card:
// a new PR defaults to P3, a procurement user raises it, the requester (staff)
// may not, and an unknown value is rejected.
func TestSetPriority(t *testing.T) {
	repo, ctx := newHandlerTestRepo(t)

	requester, err := repo.UpsertUser(ctx, "h-prio-req-"+t.Name(), "h-prio-req-"+t.Name()+"@example.com", "Requester")
	if err != nil {
		t.Fatalf("upsert requester: %v", err)
	}
	lead, err := repo.UpsertUser(ctx, "h-prio-lead-"+t.Name(), "h-prio-lead-"+t.Name()+"@example.com", "Lead")
	if err != nil {
		t.Fatalf("upsert lead: %v", err)
	}
	proc, err := repo.UpsertUser(ctx, "h-prio-proc-"+t.Name(), "h-prio-proc-"+t.Name()+"@example.com", "Procurement")
	if err != nil {
		t.Fatalf("upsert procurement: %v", err)
	}
	if err := repo.EnsureUserHasRole(ctx, proc.ID, model.RoleProcurement); err != nil {
		t.Fatalf("grant procurement role: %v", err)
	}

	h := &PurchaseRequestsHandler{Repo: repo}

	rec := httptest.NewRecorder()
	h.Create(rec, authed(requester, []string{model.RoleStaff},
		`{"title":"Priority triage","team_lead_email":"`+lead.Email+`"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d, want 201 (body=%s)", rec.Code, rec.Body.String())
	}
	var created repository.PurchaseRequest
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created PR: %v", err)
	}
	t.Cleanup(func() {
		_, _ = repo.Pool().Exec(context.Background(), `DELETE FROM purchase_requests WHERE id=$1`, created.ID)
	})
	if created.Priority != model.PriorityP3 {
		t.Fatalf("new PR priority = %q, want %q", created.Priority, model.PriorityP3)
	}

	// Before team-lead approval procurement can't see the PR, so it can't triage it.
	rec = httptest.NewRecorder()
	h.SetPriority(rec, withID(authed(proc, []string{model.RoleProcurement}, `{"priority":"P1"}`), created.ID))
	if rec.Code != http.StatusConflict {
		t.Fatalf("pre-approval set priority = %d, want 409 (body=%s)", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	h.TeamLeadDecision(rec, withID(authed(lead, []string{model.RoleStaff}, `{"decision":"approve","notes":"ok"}`), created.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("team lead approve = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}

	// Procurement raises it — no assignment needed, priority is queue triage.
	rec = httptest.NewRecorder()
	h.SetPriority(rec, withID(authed(proc, []string{model.RoleProcurement}, `{"priority":"P1"}`), created.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("set priority = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var updated repository.PurchaseRequest
	if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode updated PR: %v", err)
	}
	if updated.Priority != model.PriorityP1 {
		t.Fatalf("priority = %q, want %q", updated.Priority, model.PriorityP1)
	}
	if !updated.MyCanSetPriority {
		t.Fatal("my_can_set_priority = false for a procurement caller, want true")
	}

	// An admin (no procurement role of their own) triages too — admin passes every
	// procurement gate in this app, and priority is no exception.
	admin, err := repo.UpsertUser(ctx, "h-prio-admin-"+t.Name(), "h-prio-admin-"+t.Name()+"@example.com", "Admin")
	if err != nil {
		t.Fatalf("upsert admin: %v", err)
	}
	if err := repo.EnsureUserHasRole(ctx, admin.ID, model.RoleAdmin); err != nil {
		t.Fatalf("grant admin role: %v", err)
	}
	rec = httptest.NewRecorder()
	h.SetPriority(rec, withID(authed(admin, []string{model.RoleAdmin}, `{"priority":"P2"}`), created.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("admin set priority = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode updated PR: %v", err)
	}
	if updated.Priority != model.PriorityP2 {
		t.Fatalf("priority after admin = %q, want %q", updated.Priority, model.PriorityP2)
	}

	// The requester may not triage their own request.
	rec = httptest.NewRecorder()
	h.SetPriority(rec, withID(authed(requester, []string{model.RoleStaff}, `{"priority":"P2"}`), created.ID))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("staff set priority = %d, want 403 (body=%s)", rec.Code, rec.Body.String())
	}

	// Unknown values are rejected, not silently stored.
	rec = httptest.NewRecorder()
	h.SetPriority(rec, withID(authed(proc, []string{model.RoleProcurement}, `{"priority":"P0"}`), created.ID))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid priority = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}
}
