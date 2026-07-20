package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cs/purchasing-app/internal/middleware"
	"github.com/cs/purchasing-app/internal/model"
	"github.com/cs/purchasing-app/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

const testDSN = "postgres://chathura@localhost:5432/purchasing?sslmode=disable"

func newHandlerTestRepo(t *testing.T) (*repository.Repository, context.Context) {
	t.Helper()
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, testDSN)
	if err != nil {
		t.Skipf("no test DB: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("test DB unreachable: %v", err)
	}
	t.Cleanup(pool.Close)
	return repository.New(pool), ctx
}

// authed returns a request whose context carries the given user + roles, as the
// auth middleware would install them — so a handler can be driven directly
// without the OIDC flow.
func authed(user *repository.User, roles []string, body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	ctx := context.WithValue(r.Context(), middleware.CtxUser, user)
	ctx = context.WithValue(ctx, middleware.CtxRoles, roles)
	return r.WithContext(ctx)
}

// TestCreatePRRecordsProcessEvent drives the real Create handler and asserts a
// submit_pr process event is recorded against the new PR with the caller as
// actor — proving the handler-layer wiring, not just the repo method.
func TestCreatePRRecordsProcessEvent(t *testing.T) {
	repo, ctx := newHandlerTestRepo(t)
	user, err := repo.UpsertUser(ctx, "h-evt-"+t.Name(), "hevt@example.com", "Handler Evt")
	if err != nil {
		t.Fatalf("upsert user: %v", err)
	}

	h := &PurchaseRequestsHandler{Repo: repo}
	rec := httptest.NewRecorder()
	h.Create(rec, authed(user, []string{model.RoleStaff}, `{"title":"Test PR for events","team_lead_email":"lead@example.com"}`))

	if rec.Code != http.StatusCreated {
		t.Fatalf("create PR status = %d, body=%s", rec.Code, rec.Body.String())
	}

	// Find the PR we just created (most recent for this requester) via its events.
	var created repository.PurchaseRequest
	if err := repo.Pool().QueryRow(ctx,
		`SELECT id FROM purchase_requests WHERE requester_id = $1 ORDER BY id DESC LIMIT 1`, user.ID).
		Scan(&created.ID); err != nil {
		t.Fatalf("find created PR: %v", err)
	}

	events, err := repo.ListProcessEvents(ctx, created.ID)
	if err != nil {
		t.Fatalf("list process events: %v", err)
	}
	if len(events) != 1 || events[0].Action != model.ProcessSubmitPR {
		t.Fatalf("events = %+v, want a single submit_pr", events)
	}
	if events[0].ActorID == nil || *events[0].ActorID != user.ID || events[0].ActorEmail != user.Email {
		t.Fatalf("actor not recorded on event: %+v", events[0])
	}
}

// TestCreateVendorRecordsAuditEvent drives the real vendor Create handler and
// asserts a create_vendor audit event is recorded.
func TestCreateVendorRecordsAuditEvent(t *testing.T) {
	repo, ctx := newHandlerTestRepo(t)
	user, err := repo.UpsertUser(ctx, "h-vend-"+t.Name(), "hvend@example.com", "Vendor Evt")
	if err != nil {
		t.Fatalf("upsert user: %v", err)
	}

	// Baseline (the test is not idempotent across runs — assert a delta of 1).
	countVendorAudits := func() int {
		var n int
		if err := repo.Pool().QueryRow(ctx,
			`SELECT count(*) FROM audit_events WHERE action = $1 AND actor_id = $2`,
			model.AuditCreateVendor, user.ID).Scan(&n); err != nil {
			t.Fatalf("count audit events: %v", err)
		}
		return n
	}
	before := countVendorAudits()

	h := &VendorsHandler{Repo: repo}
	rec := httptest.NewRecorder()
	h.Create(rec, authed(user, []string{model.RoleProcurement}, `{"name":"Audit Test Vendor"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create vendor status = %d, body=%s", rec.Code, rec.Body.String())
	}

	if got := countVendorAudits() - before; got != 1 {
		t.Fatalf("create_vendor audit events added = %d, want 1", got)
	}
}
