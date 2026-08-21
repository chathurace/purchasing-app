package handler

import (
	"net/http"
	"time"

	"github.com/cs/purchasing-app/internal/middleware"
	"github.com/cs/purchasing-app/internal/model"
	"github.com/cs/purchasing-app/internal/repository"
	"github.com/rs/zerolog"
)

// SessionHandler mints and revokes the backend-issued session cookie.
//
// Why the app has its own session at all: the Asgardeo organisation is managed
// by another team, so its session and token lifetimes cannot be raised, and
// users were being signed out roughly daily. The SPA still authenticates with
// Asgardeo exactly as before — this handler exchanges the resulting ID token,
// once, for a long-lived HttpOnly cookie the browser cannot hand to JavaScript.
// From then on the app's session length is ours to set (session.ttl_days).
//
// See docs/sessions.md.
type SessionHandler struct {
	Repo    *repository.Repository
	Session middleware.SessionConfig
	Log     zerolog.Logger
}

// Create handles POST /api/v1/auth/session — the token-for-cookie exchange.
//
// It sits inside the authenticated route group, so by the time it runs the
// caller has already proven identity: either with a freshly minted Asgardeo
// Bearer token (the normal case, straight after the OIDC redirect) or with an
// existing session cookie (a no-op refresh). The auth middleware has verified
// the token and provisioned the user, so this handler adds no trust of its own —
// it only decides how long the browser may keep proving that identity without
// the IdP.
func (h *SessionHandler) Create(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromCtx(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	if !h.Session.Enabled {
		writeError(w, http.StatusNotImplemented, "cookie sessions are disabled on this server")
		return
	}

	// Already cookie-authenticated: the SPA calls this on every boot, and
	// re-minting would orphan the current session row on each page load. The
	// middleware has already slid the existing session's expiry forward.
	if existing := middleware.SessionTokenFromContext(r.Context()); existing != "" {
		writeJSON(w, http.StatusOK, sessionBody(time.Now().Add(h.Session.TTL)))
		return
	}

	token, hash, err := middleware.NewSessionToken()
	if err != nil {
		reqLog(r).Error().Err(err).Msg("session token generation failed")
		writeError(w, http.StatusInternalServerError, "could not start session")
		return
	}
	expiresAt := time.Now().Add(h.Session.TTL)
	// The ID token's `sid` ties this browser session to the IdP session behind
	// it, so a back-channel logout can end exactly this one. Empty when the IdP
	// omits the claim — revocation then falls back to sub-wide.
	idpSID := middleware.IdPSessionIDFromContext(r.Context())
	if err := h.Repo.CreateUserSession(r.Context(), hash, user.ID, expiresAt, r.UserAgent(), middleware.ClientIP(r), idpSID); err != nil {
		reqLog(r).Error().Err(err).Msg("create session")
		writeError(w, http.StatusInternalServerError, "could not start session")
		return
	}

	middleware.SetSessionCookie(w, h.Session, token, expiresAt)
	reqLog(r).Info().Time("expires_at", expiresAt).Msg("session started")
	writeJSON(w, http.StatusOK, sessionBody(expiresAt))
}

// Delete handles DELETE /api/v1/auth/session — sign out on this browser.
//
// Revoking server-side is the point: clearing the cookie alone would leave a
// token that still authenticates for the rest of its 60 days if it had already
// been captured.
func (h *SessionHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if token := middleware.SessionTokenFromContext(r.Context()); token != "" {
		if err := h.Repo.RevokeUserSession(r.Context(), middleware.HashSessionToken(token)); err != nil {
			reqLog(r).Error().Err(err).Msg("revoke session")
			writeError(w, http.StatusInternalServerError, "could not end session")
			return
		}
		reqLog(r).Info().Msg("session ended")
	}
	// Always clear the cookie, even when the request authenticated with a Bearer
	// token — logout must be idempotent from the SPA's point of view.
	middleware.ClearSessionCookie(w, h.Session)
	w.WriteHeader(http.StatusNoContent)
}

// DeleteAll handles DELETE /api/v1/auth/sessions — sign out everywhere.
//
// This is the containment lever for a long-lived cookie: a user who suspects a
// stolen laptop or browser profile can invalidate every session immediately
// instead of waiting out the TTL.
func (h *SessionHandler) DeleteAll(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromCtx(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	n, err := h.Repo.RevokeAllUserSessions(r.Context(), user.ID)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("revoke all sessions")
		writeError(w, http.StatusInternalServerError, "could not end sessions")
		return
	}
	middleware.ClearSessionCookie(w, h.Session)
	reqLog(r).Info().Int64("revoked", n).Msg("all sessions ended")
	writeJSON(w, http.StatusOK, map[string]int64{"revoked": n})
}

// DeleteForUser handles DELETE /api/v1/users/{id}/sessions — an admin ending
// someone else's sessions on every device.
//
// This is the manual half of honouring an IdP-side change. After the cookie is
// minted the IdP is never consulted again, so a disabled or offboarded Asgardeo
// account would otherwise keep working until the absolute cap
// (session.max_days) lapsed. Deactivating the user also cuts access
// immediately — it is checked on every request — but that is a heavier act; this
// signs them out without blocking a future login.
func (h *SessionHandler) DeleteForUser(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasRole(r.Context(), model.RoleAdmin) {
		writeError(w, http.StatusForbidden, "admin role required")
		return
	}
	userID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	// Reject an unknown id rather than reporting "0 revoked", which reads as
	// "that account has no sessions" and hides a typo'd id.
	if _, err := h.Repo.GetUserByID(r.Context(), userID); err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	n, err := h.Repo.RevokeAllUserSessions(r.Context(), userID)
	if err != nil {
		reqLog(r).Error().Err(err).Int64("target_user_id", userID).Msg("admin revoke sessions")
		writeError(w, http.StatusInternalServerError, "could not end sessions")
		return
	}
	recordAuditEvent(r, h.Repo, model.AuditRevokeUserSessions, model.QualifierAdmin, model.EntityUser, &userID, "")
	reqLog(r).Info().Int64("target_user_id", userID).Int64("revoked", n).Msg("admin ended user sessions")
	writeJSON(w, http.StatusOK, map[string]int64{"revoked": n})
}

// sessionBody reports the session deadline so the SPA can show it if it wants
// to; the cookie itself is set by the server and is never readable in JS.
func sessionBody(expiresAt time.Time) map[string]any {
	return map[string]any{"expires_at": expiresAt}
}
