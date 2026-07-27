package handler

import (
	"net/http"

	"github.com/cs/purchasing-app/internal/middleware"
	"github.com/cs/purchasing-app/internal/model"
	"github.com/cs/purchasing-app/internal/repository"
	"github.com/rs/zerolog"
)

// EventsHandler serves the read-only events view over the two append-only logs
// (process_events, audit_events). Writing them stays best-effort at the mutating
// handlers (see events.go); this only reads. Restricted to admin and
// procurement_admin — the log includes sensitive actions (role grants,
// deactivations), so it is not open to plain procurement/staff.
type EventsHandler struct {
	Repo *repository.Repository
	Log  zerolog.Logger
}

func (h *EventsHandler) requireAccess(w http.ResponseWriter, r *http.Request) bool {
	ctx := r.Context()
	if !middleware.HasRole(ctx, model.RoleAdmin) && !middleware.HasRole(ctx, model.RoleProcurementAdmin) {
		writeError(w, http.StatusForbidden, "admin or procurement_admin access required")
		return false
	}
	return true
}

// eventFilter reads the shared query-string filters (actor/action/from/to/limit,
// plus the process-only pr_id).
func eventFilter(r *http.Request) repository.EventFilter {
	q := r.URL.Query()
	f := repository.EventFilter{
		ActorTerm: q.Get("actor"),
		Action:    q.Get("action"),
		From:      q.Get("from"),
		To:        q.Get("to"),
		PRID:      parseInt64Param(q.Get("pr_id")),
	}
	if lim := parseInt64Param(q.Get("limit")); lim != nil {
		f.Limit = int(*lim)
	}
	return f
}

// ListProcess returns process events newest-first across all PRs, narrowed by the
// filter (admin / procurement_admin).
func (h *EventsHandler) ListProcess(w http.ResponseWriter, r *http.Request) {
	if !h.requireAccess(w, r) {
		return
	}
	events, err := h.Repo.FilterProcessEvents(r.Context(), eventFilter(r))
	if err != nil {
		reqLog(r).Error().Err(err).Msg("list process events")
		writeError(w, http.StatusInternalServerError, "failed to list process events")
		return
	}
	if events == nil {
		events = []repository.ProcessEvent{}
	}
	writeJSON(w, http.StatusOK, events)
}

// ListAudit returns audit (master-data/admin) events newest-first, narrowed by the
// filter (admin / procurement_admin).
func (h *EventsHandler) ListAudit(w http.ResponseWriter, r *http.Request) {
	if !h.requireAccess(w, r) {
		return
	}
	events, err := h.Repo.FilterAuditEvents(r.Context(), eventFilter(r))
	if err != nil {
		reqLog(r).Error().Err(err).Msg("list audit events")
		writeError(w, http.StatusInternalServerError, "failed to list audit events")
		return
	}
	if events == nil {
		events = []repository.AuditEvent{}
	}
	writeJSON(w, http.StatusOK, events)
}

// Actions returns the fixed action catalogs for the two logs, so the view's
// action-filter dropdowns stay in sync with the Go source of truth (admin / procurement_admin).
func (h *EventsHandler) Actions(w http.ResponseWriter, r *http.Request) {
	if !h.requireAccess(w, r) {
		return
	}
	writeJSON(w, http.StatusOK, map[string][]string{
		"process": model.SortedProcessActions(),
		"audit":   model.SortedAuditActions(),
	})
}
