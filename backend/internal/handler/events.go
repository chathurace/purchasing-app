package handler

import (
	"net/http"

	"github.com/cs/purchasing-app/internal/middleware"
	"github.com/cs/purchasing-app/internal/repository"
)

// recordProcessEvent appends a PR business-process event, best-effort: it runs
// AFTER the mutation has succeeded, and a failure is logged (with the
// request-scoped logger) but never affects the response — the action already
// happened. The actor is taken from the request context, so it is captured even
// for the few repo funcs that drop the actor id.
func recordProcessEvent(r *http.Request, repo *repository.Repository, prID int64, action, qualifier string) {
	id, email := actor(r)
	if err := repo.AddProcessEvent(r.Context(), prID, action, qualifier, id, email); err != nil {
		reqLog(r).Error().Err(err).Str("action", action).Str("qualifier", qualifier).
			Int64("pr_id", prID).Msg("record process event")
	}
}

// recordAuditEvent appends a non-process (master-data/admin) event. Same
// best-effort, logged-on-failure contract as recordProcessEvent.
func recordAuditEvent(r *http.Request, repo *repository.Repository, action, qualifier, entityType string, entityID *int64, detail string) {
	id, email := actor(r)
	if err := repo.AddAuditEvent(r.Context(), action, qualifier, entityType, entityID, detail, id, email); err != nil {
		reqLog(r).Error().Err(err).Str("action", action).Str("entity_type", entityType).
			Msg("record audit event")
	}
}

// actor returns the acting user's id and email from the request context (0/""
// if somehow absent — the event still records with a NULL actor).
func actor(r *http.Request) (int64, string) {
	u := middleware.UserFromCtx(r.Context())
	if u == nil {
		return 0, ""
	}
	return u.ID, u.Email
}
