package handler

import (
	"errors"
	"net/http"

	"github.com/cs/purchasing-app/internal/middleware"
	"github.com/cs/purchasing-app/internal/model"
	"github.com/cs/purchasing-app/internal/repository"
	"github.com/rs/zerolog"
)

// BackchannelLogoutHandler receives the IdP's OIDC back-channel logout callback.
//
// This is how an IdP-side sign-out reaches the app. Once a session cookie is
// minted the IdP is never consulted again (that is the point — it is what makes
// sessions survive token expiry), so without this callback a user signing out at
// Asgardeo, or an admin terminating their IdP session, would leave the
// purchasing session running for up to session.max_days.
//
// The route is deliberately OUTSIDE the authenticated group: the caller is the
// IdP's server, not a browser, and it carries no cookie or Bearer token. The
// signed logout token *is* the credential — see middleware.VerifyLogoutToken.
//
// Front-channel logout was not used: it only fires while the app is open in a
// browser tab, so it would miss exactly the offboarding case this exists for.
type BackchannelLogoutHandler struct {
	Repo *repository.Repository
	Auth *middleware.AuthMiddleware
	Log  zerolog.Logger
}

// Post handles POST /api/v1/auth/backchannel-logout.
//
// Per OIDC Back-Channel Logout 1.0 §2.8 the response carries no body, must not
// be cached, and reports 400 for a token that cannot be validated. Notably it
// returns **200 even when nothing was revoked**: the IdP broadcasts to every
// registered app, so "this subject never signed in here" is a normal outcome and
// not a failure the IdP should retry.
func (h *BackchannelLogoutHandler) Post(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")

	if !h.Auth.BackchannelLogoutReady() {
		reqLog(r).Error().Msg("back-channel logout received but no IdP key set is available")
		writeError(w, http.StatusServiceUnavailable, "logout verification unavailable")
		return
	}
	// The token arrives form-encoded as `logout_token` (§2.5).
	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid form body")
		return
	}
	raw := r.PostFormValue("logout_token")
	if raw == "" {
		writeError(w, http.StatusBadRequest, "logout_token is required")
		return
	}

	token, err := h.Auth.VerifyLogoutToken(r.Context(), raw)
	if err != nil {
		// Log the reason but never echo it: the caller is either the IdP (which
		// cannot act on the detail) or someone probing what we accept.
		reqLog(r).Warn().Err(err).Str("ip", middleware.ClientIP(r)).Msg("rejected back-channel logout token")
		if errors.Is(err, middleware.ErrNoLogoutSubject) {
			writeError(w, http.StatusBadRequest, "logout_token names neither a session nor a subject")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid logout_token")
		return
	}

	// Prefer sid: it names one IdP session, so only the browser whose session
	// ended is signed out. Fall back to sub — meaning "this person's session is
	// over" — when the IdP sent no sid, or sent one from before this app started
	// recording them.
	var revoked, userID int64
	scope := "sid"
	if token.SessionID != "" {
		revoked, userID, err = h.Repo.RevokeUserSessionsByIdPSID(r.Context(), token.SessionID)
	}
	if err == nil && revoked == 0 && token.Subject != "" {
		scope = "sub"
		revoked, userID, err = h.Repo.RevokeUserSessionsBySub(r.Context(), token.Subject)
	}
	if err != nil {
		reqLog(r).Error().Err(err).Msg("back-channel logout revoke")
		writeError(w, http.StatusInternalServerError, "could not end sessions")
		return
	}

	if revoked > 0 && userID > 0 {
		// Audited like the admin action, with the qualifier saying who ended it.
		// The actor is the IdP, not a signed-in user, so the event's actor is
		// NULL — recordAuditEvent handles that.
		recordAuditEvent(r, h.Repo, model.AuditRevokeUserSessions, model.QualifierIdPLogout, model.EntityUser, &userID, "back-channel logout")
	}
	reqLog(r).Info().
		Str("scope", scope).
		Int64("revoked", revoked).
		Int64("user_id", userID).
		Bool("had_sid", token.SessionID != "").
		Msg("back-channel logout processed")
	w.WriteHeader(http.StatusOK)
}
