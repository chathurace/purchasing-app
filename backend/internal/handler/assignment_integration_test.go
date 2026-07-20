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
