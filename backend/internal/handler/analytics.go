package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/cs/purchasing-app/internal/repository"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"
)

// AnalyticsHandler serves the BPM analytics views (docs/bpm-analytics.md): the
// cross-organisation purchase-request list and, per request, its process-event
// flow. Read-only.
//
// Gated exactly like the audit log (hasEventLogAccess — admin /
// procurement_admin), because the flow view returns the same process_events rows
// the audit log does. That gate is also *why* these reads apply no per-PR
// visibility rule: the audience already sees every PR in the log.
type AnalyticsHandler struct {
	Repo *repository.Repository
	Log  zerolog.Logger
}

func (h *AnalyticsHandler) requireAccess(w http.ResponseWriter, r *http.Request) bool {
	if !hasEventLogAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "admin or procurement_admin access required")
		return false
	}
	return true
}

// PRFlowResponse is the per-PR analytics page: the request header plus its whole
// process-event timeline, oldest-first. One round trip, and independent of the
// gated PR read (a procurement_admin may analyse a request they could not open).
type PRFlowResponse struct {
	PurchaseRequest *repository.AnalyticsPR   `json:"purchase_request"`
	Events          []repository.ProcessEvent `json:"events"`
}

// analyticsSort maps the ?sort= value to a whitelisted column, and ?dir= to a
// direction. An unknown column falls back to created_at; an unknown direction to
// the column's natural default — newest-first for a timestamp, A→Z for a person,
// which is what each header reads as on first click.
func analyticsSort(r *http.Request) (repository.AnalyticsPRSort, bool) {
	sort := repository.AnalyticsPRSort(r.URL.Query().Get("sort"))
	switch sort {
	case repository.AnalyticsPRSortRequester, repository.AnalyticsPRSortAssignee:
	default:
		sort = repository.AnalyticsPRSortCreated
	}
	desc := sort == repository.AnalyticsPRSortCreated
	switch r.URL.Query().Get("dir") {
	case "asc":
		desc = false
	case "desc":
		desc = true
	}
	return sort, desc
}

// ListPRs returns every purchase request for the analytics list, sorted by the
// requested column and capped at a page (repository.analyticsPRLimit).
func (h *AnalyticsHandler) ListPRs(w http.ResponseWriter, r *http.Request) {
	if !h.requireAccess(w, r) {
		return
	}
	sort, desc := analyticsSort(r)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit")) // 0 (or junk) = server default
	prs, err := h.Repo.ListAnalyticsPRs(r.Context(), sort, desc, limit)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("list analytics purchase requests")
		writeError(w, http.StatusInternalServerError, "failed to list purchase requests")
		return
	}
	if prs == nil {
		prs = []*repository.AnalyticsPR{}
	}
	writeJSON(w, http.StatusOK, prs)
}

// PRFlow returns one request's header and its full process-event timeline.
func (h *AnalyticsHandler) PRFlow(w http.ResponseWriter, r *http.Request) {
	if !h.requireAccess(w, r) {
		return
	}
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid purchase request id")
		return
	}
	pr, err := h.Repo.GetAnalyticsPR(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "purchase request not found")
			return
		}
		reqLog(r).Error().Err(err).Msg("get analytics purchase request")
		writeError(w, http.StatusInternalServerError, "failed to load purchase request")
		return
	}
	events, err := h.Repo.ListProcessEvents(r.Context(), id)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("list process events for pr")
		writeError(w, http.StatusInternalServerError, "failed to load process events")
		return
	}
	if events == nil {
		events = []repository.ProcessEvent{}
	}
	writeJSON(w, http.StatusOK, PRFlowResponse{PurchaseRequest: pr, Events: events})
}
