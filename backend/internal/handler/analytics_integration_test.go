package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cs/purchasing-app/internal/middleware"
	"github.com/cs/purchasing-app/internal/model"
	"github.com/cs/purchasing-app/internal/repository"
)

// authedGet returns an authenticated GET request for `url` (so query-string
// filters and sort params can be driven), mirroring authed() for the read-only
// analytics handlers.
func authedGet(user *repository.User, roles []string, url string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, url, nil)
	ctx := context.WithValue(r.Context(), middleware.CtxUser, user)
	ctx = context.WithValue(ctx, middleware.CtxRoles, roles)
	return r.WithContext(ctx)
}

// findAnalyticsPR locates a PR by id in a decoded analytics list, or nil.
func findAnalyticsPR(list []repository.AnalyticsPR, id int64) *repository.AnalyticsPR {
	for i := range list {
		if list[i].ID == id {
			return &list[i]
		}
	}
	return nil
}

// TestAnalyticsAccessIsAdminOnly asserts the analytics reads are gated to
// admin / procurement_admin — the same audience as the audit log, since the flow
// view returns the very same process_events rows.
func TestAnalyticsAccessIsAdminOnly(t *testing.T) {
	repo, _ := newHandlerTestRepo(t)
	h := &AnalyticsHandler{Repo: repo}

	denied := [][]string{
		{model.RoleStaff},
		{model.RoleProcurement},
		{model.RoleLegal},
	}
	for _, roles := range denied {
		rec := httptest.NewRecorder()
		h.ListPRs(rec, authedGet(&repository.User{ID: 1}, roles, "/"))
		if rec.Code != http.StatusForbidden {
			t.Errorf("ListPRs as %v = %d, want 403", roles, rec.Code)
		}
	}
	for _, roles := range [][]string{{model.RoleAdmin}, {model.RoleProcurementAdmin}} {
		rec := httptest.NewRecorder()
		h.ListPRs(rec, authedGet(&repository.User{ID: 1}, roles, "/"))
		if rec.Code != http.StatusOK {
			t.Errorf("ListPRs as %v = %d, want 200 (body=%s)", roles, rec.Code, rec.Body.String())
		}
	}
}

// TestAnalyticsPRFlow drives both analytics reads end-to-end over a real PR: it
// appears in the list even though its team lead has not approved it (analytics
// applies no visibility gate), and its flow returns the header plus the process
// events oldest-first with the actor's name resolved.
func TestAnalyticsPRFlow(t *testing.T) {
	repo, ctx := newHandlerTestRepo(t)

	requester, err := repo.UpsertUser(ctx, "h-an-req-"+t.Name(), "h-an-req-"+t.Name()+"@example.com", "Ana Requester")
	if err != nil {
		t.Fatalf("upsert requester: %v", err)
	}

	prh := &PurchaseRequestsHandler{Repo: repo}
	rec := httptest.NewRecorder()
	prh.Create(rec, authed(requester, []string{model.RoleStaff},
		`{"title":"Analytics flow PR","team_lead_email":"an-lead@example.com"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create PR = %d (body=%s)", rec.Code, rec.Body.String())
	}
	var created repository.PurchaseRequest
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created PR: %v", err)
	}
	// One cleanup for both rows so the order is explicit: the PR first, then the
	// requester it references.
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = repo.Pool().Exec(bg, `DELETE FROM purchase_requests WHERE id=$1`, created.ID)
		_, _ = repo.Pool().Exec(bg, `DELETE FROM users WHERE id=$1`, requester.ID)
	})

	admin := &repository.User{ID: requester.ID, Email: requester.Email}
	h := &AnalyticsHandler{Repo: repo}

	// The PR's team lead has not approved, so procurement cannot see it on the
	// normal list — but analytics is ungated and must still list it.
	rec = httptest.NewRecorder()
	h.ListPRs(rec, authedGet(admin, []string{model.RoleProcurementAdmin}, "/?sort=created_at&dir=desc"))
	if rec.Code != http.StatusOK {
		t.Fatalf("ListPRs = %d (body=%s)", rec.Code, rec.Body.String())
	}
	var list []repository.AnalyticsPR
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	row := findAnalyticsPR(list, created.ID)
	if row == nil {
		t.Fatalf("PR %d missing from the analytics list (%d rows)", created.ID, len(list))
	}
	if row.Title != "Analytics flow PR" || row.Priority != "P3" {
		t.Errorf("row = %+v, want title/priority of the created PR", row)
	}
	if row.Requester == nil || row.Requester.Name != "Ana Requester" {
		t.Errorf("row.Requester = %+v, want the resolved requester", row.Requester)
	}
	if row.Assignee != nil {
		t.Errorf("row.Assignee = %+v, want nil while unassigned", row.Assignee)
	}

	// The flow read: header + the submit_pr event the create recorded.
	rec = httptest.NewRecorder()
	h.PRFlow(rec, withID(authedGet(admin, []string{model.RoleAdmin}, "/"), created.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("PRFlow = %d (body=%s)", rec.Code, rec.Body.String())
	}
	var flow PRFlowResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &flow); err != nil {
		t.Fatalf("decode flow: %v", err)
	}
	if flow.PurchaseRequest == nil || flow.PurchaseRequest.ID != created.ID {
		t.Fatalf("flow header = %+v, want PR %d", flow.PurchaseRequest, created.ID)
	}
	if len(flow.Events) != 1 || flow.Events[0].Action != model.ProcessSubmitPR {
		t.Fatalf("flow events = %+v, want a single submit_pr", flow.Events)
	}
	// ActorName is what the timeline renders; ListProcessEvents must join it.
	if flow.Events[0].ActorName != "Ana Requester" {
		t.Errorf("event actor_name = %q, want the joined display name", flow.Events[0].ActorName)
	}

	// An unknown id is a 404, not an empty flow.
	rec = httptest.NewRecorder()
	h.PRFlow(rec, withID(authedGet(admin, []string{model.RoleAdmin}, "/"), -1))
	if rec.Code != http.StatusNotFound {
		t.Errorf("PRFlow on unknown id = %d, want 404", rec.Code)
	}
}

// TestAnalyticsPRSorting asserts the whitelisted sort columns actually reorder
// the list, that an unknown column falls back to created_at rather than erroring,
// and that unassigned PRs sort last in both directions of the assignee column.
func TestAnalyticsPRSorting(t *testing.T) {
	repo, ctx := newHandlerTestRepo(t)
	h := &AnalyticsHandler{Repo: repo}
	admin := &repository.User{ID: 1}

	get := func(t *testing.T, query string) []repository.AnalyticsPR {
		t.Helper()
		rec := httptest.NewRecorder()
		h.ListPRs(rec, authedGet(admin, []string{model.RoleAdmin}, "/"+query))
		if rec.Code != http.StatusOK {
			t.Fatalf("ListPRs%s = %d (body=%s)", query, rec.Code, rec.Body.String())
		}
		var list []repository.AnalyticsPR
		if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return list
	}

	var total int
	if err := repo.Pool().QueryRow(ctx, `SELECT count(*) FROM purchase_requests`).Scan(&total); err != nil {
		t.Fatalf("count PRs: %v", err)
	}
	if total < 2 {
		t.Skip("need at least two purchase requests to observe an ordering")
	}

	desc := get(t, "?sort=created_at&dir=desc")
	asc := get(t, "?sort=created_at&dir=asc")
	if len(desc) < 2 || len(asc) < 2 {
		t.Skip("fewer than two rows returned")
	}
	if !desc[0].CreatedAt.After(desc[len(desc)-1].CreatedAt) {
		t.Errorf("created_at desc is not newest-first: %v .. %v", desc[0].CreatedAt, desc[len(desc)-1].CreatedAt)
	}
	if asc[0].ID == desc[0].ID && !asc[0].CreatedAt.Equal(desc[0].CreatedAt) {
		t.Errorf("dir=asc did not reverse the order")
	}

	// An unknown sort column must not error — it falls back to created_at desc.
	if bogus := get(t, "?sort=drop_table&dir=sideways"); len(bogus) == 0 || bogus[0].ID != desc[0].ID {
		t.Errorf("unknown sort did not fall back to the created_at default")
	}

	// Unassigned PRs sort last whichever way the assignee column points.
	for _, dir := range []string{"asc", "desc"} {
		rows := get(t, "?sort=assignee&dir="+dir)
		seenUnassigned := false
		for _, r := range rows {
			if r.Assignee == nil {
				seenUnassigned = true
			} else if seenUnassigned {
				t.Errorf("assignee %s: PR %d (assigned) sorts after an unassigned row", dir, r.ID)
				break
			}
		}
	}
}
