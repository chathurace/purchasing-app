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
	"github.com/go-chi/chi/v5"
)

// withID attaches a chi route context carrying {id}, so a handler that reads
// chi.URLParam(r, "id") can be driven directly without a router.
func withID(r *http.Request, id int64) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", strconv.FormatInt(id, 10))
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// TestTeamLeadApprovalFlow drives the real HTTP handlers end-to-end: a PR is
// created with a team lead, is invisible to procurement and blocks quotation creation
// while pending, and both open up once the team lead approves.
func TestTeamLeadApprovalFlow(t *testing.T) {
	repo, ctx := newHandlerTestRepo(t)

	requester, err := repo.UpsertUser(ctx, "h-tl-req-"+t.Name(), "h-tl-req-"+t.Name()+"@example.com", "Requester")
	if err != nil {
		t.Fatalf("upsert requester: %v", err)
	}
	lead, err := repo.UpsertUser(ctx, "h-tl-lead-"+t.Name(), "h-tl-lead-"+t.Name()+"@example.com", "Lead")
	if err != nil {
		t.Fatalf("upsert lead: %v", err)
	}
	fin, err := repo.UpsertUser(ctx, "h-tl-fin-"+t.Name(), "h-tl-fin-"+t.Name()+"@example.com", "Procurement")
	if err != nil {
		t.Fatalf("upsert procurement: %v", err)
	}
	// The assignee/collaborator validation checks the DB role, so grant it there
	// (the request-context roles from authed() are not enough for self-assign).
	if err := repo.EnsureUserHasRole(ctx, fin.ID, model.RoleProcurement); err != nil {
		t.Fatalf("grant procurement role: %v", err)
	}

	h := &PurchaseRequestsHandler{Repo: repo}

	// A team lead email is required.
	rec := httptest.NewRecorder()
	h.Create(rec, authed(requester, []string{model.RoleStaff}, `{"title":"No lead"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("create without team lead = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}

	// Create the PR with the team lead.
	rec = httptest.NewRecorder()
	h.Create(rec, authed(requester, []string{model.RoleStaff},
		`{"title":"Gated PR","team_lead_email":"`+lead.Email+`"}`))
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

	// Procurement cannot view the pending PR.
	rec = httptest.NewRecorder()
	h.Get(rec, withID(authed(fin, []string{model.RoleProcurement}, ""), created.ID))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("procurement Get pending = %d, want 403 (body=%s)", rec.Code, rec.Body.String())
	}

	// Quotation creation is blocked (409) while pending.
	qh := &QuotationsHandler{Repo: repo}
	rec = httptest.NewRecorder()
	qh.Create(rec, withID(authed(fin, []string{model.RoleProcurement}, `{"vendor_id":1}`), created.ID))
	if rec.Code != http.StatusConflict {
		t.Fatalf("quotation create pending = %d, want 409 (body=%s)", rec.Code, rec.Body.String())
	}

	// A non-team-lead cannot decide.
	rec = httptest.NewRecorder()
	h.TeamLeadDecision(rec, withID(authed(fin, []string{model.RoleProcurement}, `{"decision":"approve"}`), created.ID))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("procurement decision = %d, want 403 (body=%s)", rec.Code, rec.Body.String())
	}

	// The team lead approves.
	rec = httptest.NewRecorder()
	h.TeamLeadDecision(rec, withID(authed(lead, []string{model.RoleStaff}, `{"decision":"approve","notes":"lgtm"}`), created.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("team lead approve = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}

	// Procurement can now view it.
	rec = httptest.NewRecorder()
	h.Get(rec, withID(authed(fin, []string{model.RoleProcurement}, ""), created.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("procurement Get after approval = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}

	// Team-lead-approved but still unassigned: quotation creation is blocked by the
	// assignment gate (409).
	rec = httptest.NewRecorder()
	qh.Create(rec, withID(authed(fin, []string{model.RoleProcurement}, `{"vendor_id":1}`), created.ID))
	if rec.Code != http.StatusConflict {
		t.Fatalf("quotation create before assignment = %d, want 409 (body=%s)", rec.Code, rec.Body.String())
	}

	// A procurement user claims (self-assigns) the PR.
	rec = httptest.NewRecorder()
	h.SetAssignee(rec, withID(authed(fin, []string{model.RoleProcurement}, `{"assignee_id":`+strconv.FormatInt(fin.ID, 10)+`}`), created.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("self-assign = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	// The assignment is recorded as an assign_pr process event (best-effort write —
	// this also confirms the action is registered in ValidProcessActions).
	var events int
	if err := repo.Pool().QueryRow(ctx,
		`SELECT count(*) FROM process_events WHERE purchase_request_id=$1 AND action='assign_pr' AND qualifier='assign'`,
		created.ID).Scan(&events); err != nil {
		t.Fatalf("count assign_pr events: %v", err)
	}
	if events != 1 {
		t.Fatalf("assign_pr process events = %d, want 1", events)
	}

	// The assignee gets past both gates now (fails later on the missing vendor instead).
	rec = httptest.NewRecorder()
	qh.Create(rec, withID(authed(fin, []string{model.RoleProcurement}, `{"vendor_id":0}`), created.ID))
	if rec.Code == http.StatusConflict {
		t.Fatalf("quotation create after assignment still 409 (body=%s)", rec.Body.String())
	}

	// A different procurement user who is neither assignee nor collaborator may now
	// also work on the assigned PR — assignment tracks the owner, it does not lock
	// others out — and doing non-readonly work auto-enrolls them as a collaborator.
	other, err := repo.UpsertUser(ctx, "h-tl-other-"+t.Name(), "h-tl-other-"+t.Name()+"@example.com", "Other")
	if err != nil {
		t.Fatalf("upsert other: %v", err)
	}
	vendor, err := repo.CreateVendor(ctx, repository.VendorInput{Name: "Acme " + t.Name(), IsActive: true}, fin.ID)
	if err != nil {
		t.Fatalf("create vendor: %v", err)
	}
	t.Cleanup(func() {
		// Runs before the PR cleanup (LIFO): drop the vendor's quotations first so
		// the vendor row is no longer referenced, then the vendor.
		_, _ = repo.Pool().Exec(context.Background(), `DELETE FROM quotations WHERE vendor_id=$1`, vendor.ID)
		_, _ = repo.Pool().Exec(context.Background(), `DELETE FROM vendors WHERE id=$1`, vendor.ID)
	})
	rec = httptest.NewRecorder()
	qh.Create(rec, withID(authed(other, []string{model.RoleProcurement},
		`{"vendor_id":`+strconv.FormatInt(vendor.ID, 10)+`}`), created.ID))
	if rec.Code != http.StatusCreated {
		t.Fatalf("other procurement user quotation create = %d, want 201 (body=%s)", rec.Code, rec.Body.String())
	}
	// They are now a collaborator on the PR.
	var isCollab bool
	if err := repo.Pool().QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM purchase_request_collaborators WHERE purchase_request_id=$1 AND user_id=$2)`,
		created.ID, other.ID).Scan(&isCollab); err != nil {
		t.Fatalf("check collaborator: %v", err)
	}
	if !isCollab {
		t.Fatalf("expected user %d to be auto-added as a collaborator after doing work", other.ID)
	}
}
