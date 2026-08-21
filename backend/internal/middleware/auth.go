package middleware

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/cs/purchasing-app/internal/model"
	"github.com/cs/purchasing-app/internal/repository"
	"github.com/rs/zerolog"
)

// unverifiedTokenClaims decodes a JWT's payload WITHOUT verifying its signature.
// Diagnostics only — never trust these values. Used to log what the incoming
// token actually carries (iss/aud/exp) when verification fails, so a config
// mismatch is obvious from the logs.
func unverifiedTokenClaims(raw string) map[string]any {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	var m map[string]any
	if json.Unmarshal(payload, &m) != nil {
		return nil
	}
	return m
}

type contextKey string

const (
	CtxUser  contextKey = "user"
	CtxRoles contextKey = "roles"
)

type AuthMiddleware struct {
	verifier            *oidc.IDTokenVerifier
	repo                *repository.Repository
	bootstrapAdminEmail string
	session             SessionConfig
	log                 zerolog.Logger
	// keySet / issuer / clientID back VerifyLogoutToken (see logout_token.go).
	// A back-channel logout token is a different artifact from an ID token, so it
	// cannot go through `verifier` — we verify its signature against the same
	// JWKS and check its own claim rules.
	keySet   oidc.KeySet
	issuer   string
	clientID string
}

// AuthConfig carries the OIDC settings (sourced from config.yaml).
type AuthConfig struct {
	Issuer              string
	DiscoveryURL        string
	ClientID            string
	InsecureSkipVerify  bool
	BootstrapAdminEmail string
	// Session is the backend-issued cookie session config (see session.go).
	// A zero value (Enabled false) leaves the middleware Bearer-only, exactly
	// as it behaved before cookie sessions existed.
	Session SessionConfig
}

func NewAuth(ctx context.Context, cfg AuthConfig, repo *repository.Repository, log zerolog.Logger) (*AuthMiddleware, error) {
	if cfg.InsecureSkipVerify {
		log.Warn().Msg("OIDC TLS verification disabled (dev mode)")
		client := &http.Client{Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}}
		ctx = oidc.ClientContext(ctx, client)
	}
	// go-oidc appends "/.well-known/openid-configuration" to the URL it is
	// given, so we pass the issuer *base*, not the full discovery URL. Allow a
	// discovery_url override (some IdPs host discovery under a different base
	// than the issuer claim); tolerate a value that already includes the
	// well-known suffix by stripping it. Only engage InsecureIssuerURLContext
	// when the discovery base genuinely differs from the issuer claim.
	discoveryURL := cfg.Issuer
	if cfg.DiscoveryURL != "" {
		base := strings.TrimSuffix(strings.TrimRight(cfg.DiscoveryURL, "/"), "/.well-known/openid-configuration")
		if base != cfg.Issuer {
			ctx = oidc.InsecureIssuerURLContext(ctx, cfg.Issuer)
		}
		discoveryURL = base
	}
	provider, err := oidc.NewProvider(ctx, discoveryURL)
	if err != nil {
		return nil, err
	}
	verifier := provider.Verifier(&oidc.Config{ClientID: cfg.ClientID})
	// Key set for back-channel logout tokens. A failure here is not fatal: the
	// app authenticates fine without it, only the logout callback is unavailable
	// (and it reports that), so a discovery quirk must not stop the server.
	keySet, jwksURL, keyErr := keySetFromProvider(ctx, provider)
	if keyErr != nil {
		log.Warn().Err(keyErr).Msg("back-channel logout unavailable: could not resolve jwks_uri")
	}
	// Log the EFFECTIVE OIDC config the running server loaded, so "is the deploy
	// actually using my config.yaml?" is answerable from the boot logs alone.
	log.Info().
		Str("issuer", cfg.Issuer).
		Str("discovery_url", discoveryURL).
		Str("client_id", cfg.ClientID).
		Str("jwks_url", jwksURL).
		Bool("insecure_skip_verify", cfg.InsecureSkipVerify).
		Msg("OIDC initialized")
	if cfg.BootstrapAdminEmail != "" {
		log.Info().Str("email", cfg.BootstrapAdminEmail).Msg("bootstrap admin configured")
	}
	return &AuthMiddleware{
		verifier:            verifier,
		repo:                repo,
		bootstrapAdminEmail: cfg.BootstrapAdminEmail,
		session:             cfg.Session,
		log:                 log,
		keySet:              keySet,
		issuer:              cfg.Issuer,
		clientID:            cfg.ClientID,
	}, nil
}

// BackchannelLogoutReady reports whether logout tokens can be verified — false
// when the IdP's jwks_uri could not be resolved at startup.
func (a *AuthMiddleware) BackchannelLogoutReady() bool { return a.keySet != nil }

// SessionConfig exposes the session settings the middleware was built with, so
// the session handler and the router share exactly one copy.
func (a *AuthMiddleware) SessionConfig() SessionConfig { return a.session }

func (a *AuthMiddleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Request-scoped logger installed by middleware.RequestLogger — carries
		// the request_id, and (once we resolve the caller below) the user.
		reqLog := zerolog.Ctx(r.Context())

		// Backend-issued session cookie (migration 052, docs/sessions.md).
		// Checked BEFORE the Bearer path because it is how the SPA authenticates
		// for the whole life of a session — the IdP token is presented exactly
		// once, at POST /api/v1/auth/session, to mint this cookie. A stale
		// Bearer token the browser still happens to hold is therefore ignored
		// rather than rejected.
		if a.session.Enabled {
			if a.authenticateSession(w, r, next, reqLog) {
				return
			}
		}

		rawToken := extractBearerToken(r)
		if rawToken == "" {
			reqLog.Warn().Msg("auth rejected: missing authorization header")
			http.Error(w, "missing authorization header", http.StatusUnauthorized)
			return
		}

		idToken, err := a.verifier.Verify(r.Context(), rawToken)
		if err != nil {
			// Put the reason AND the (unverified) token's iss/aud/exp directly in
			// the message string — Choreo's log viewer drops structured fields, so
			// the message is the only reliably-visible channel. Claims are
			// untrusted; logging only.
			c := unverifiedTokenClaims(rawToken)
			reqLog.Warn().Msg(fmt.Sprintf(
				"token verification failed: %v | token iss=%v aud=%v exp=%v",
				err, c["iss"], c["aud"], c["exp"]))
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		var claims struct {
			Sub               string `json:"sub"`
			Email             string `json:"email"`
			Name              string `json:"name"`
			PreferredUsername string `json:"preferred_username"`
			Username          string `json:"username"`
			// SID is the IdP session this token came from. Recorded on the
			// session row at mint time so a back-channel logout naming that sid
			// can revoke exactly this browser (see logout_token.go).
			SID string `json:"sid"`
		}
		if err := idToken.Claims(&claims); err != nil {
			reqLog.Warn().Err(err).Msg("auth rejected: invalid token claims")
			http.Error(w, "invalid token claims", http.StatusUnauthorized)
			return
		}

		name := firstNonEmpty(claims.Name, claims.PreferredUsername, claims.Username)
		// Find an email-like value across the common claim locations.
		email := claims.Email
		if email == "" {
			for _, c := range []string{claims.PreferredUsername, claims.Username, claims.Sub} {
				if strings.Contains(c, "@") {
					email = c
					break
				}
			}
		}

		reqLog.Debug().
			Str("sub", claims.Sub).
			Str("email", email).
			Str("preferred_username", claims.PreferredUsername).
			Str("username", claims.Username).
			Msg("authenticated user")

		// Resolve the identity: known sub, a pending invite claimed by email, or
		// a brand-new self-provisioned user (see repo.ProvisionUserOnLogin).
		user, err := a.repo.ProvisionUserOnLogin(r.Context(), claims.Sub, email, name)
		if err != nil {
			reqLog.Error().Err(err).Msg("provision user")
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		// Bootstrap admin match: any of the claims that may carry the configured
		// identity (email, username, or sub).
		isBootstrapAdmin := a.bootstrapAdminEmail != "" &&
			(email == a.bootstrapAdminEmail ||
				claims.PreferredUsername == a.bootstrapAdminEmail ||
				claims.Username == a.bootstrapAdminEmail ||
				claims.Sub == a.bootstrapAdminEmail)

		// Carry the token's sid so POST /auth/session can stamp it on the row it
		// creates. Only the Bearer path has it — a cookie-authenticated request
		// presents no token.
		ctx := ContextWithIdPSessionID(r.Context(), claims.SID)
		a.serveAuthenticated(w, r.WithContext(ctx), next, reqLog, user, "", isBootstrapAdmin)
	})
}

// authenticateSession resolves a request carrying a backend-issued session
// cookie. It reports handled=true once it has either served the request or
// written a response, so the caller must return immediately. handled=false means
// "no cookie presented" — fall through to the Bearer path.
func (a *AuthMiddleware) authenticateSession(w http.ResponseWriter, r *http.Request, next http.Handler, reqLog *zerolog.Logger) (handled bool) {
	token := SessionTokenFromRequest(r, a.session)
	if token == "" {
		return false
	}

	// CSRF: cookies ride along automatically on cross-site requests, so writes
	// must additionally prove same-origin. See CheckCSRFOrigin.
	if !CheckCSRFOrigin(r, a.session.AllowedOrigins) {
		reqLog.Warn().Str("origin", r.Header.Get("Origin")).Msg("cookie-authenticated write rejected: cross-origin request")
		http.Error(w, "cross-origin request rejected", http.StatusForbidden)
		return true
	}

	userID, err := a.repo.TouchUserSession(r.Context(), HashSessionToken(token), a.session.TTL, a.session.MaxLifetime)
	if err != nil {
		// Unknown, expired, or revoked. Clear the cookie so the browser stops
		// resending a dead token on every request until it ages out.
		if !errors.Is(err, repository.ErrSessionInvalid) {
			reqLog.Error().Err(err).Msg("session lookup failed")
		}
		ClearSessionCookie(w, a.session)
		http.Error(w, "session expired", http.StatusUnauthorized)
		return true
	}

	user, err := a.repo.GetUserByID(r.Context(), userID)
	if err != nil || user == nil {
		// The session outlived its user (deleted account). ON DELETE CASCADE
		// normally removes the row first, so this is the belt-and-braces branch.
		ClearSessionCookie(w, a.session)
		http.Error(w, "session expired", http.StatusUnauthorized)
		return true
	}

	a.serveAuthenticated(w, r, next, reqLog, user, token,
		a.bootstrapAdminEmail != "" && user.Email == a.bootstrapAdminEmail)
	return true
}

// serveAuthenticated is the tail both credential paths share: the deactivation
// gate, default-role provisioning, the bootstrap-admin grant, and the role
// lookup that populates the request context. sessionToken is the cookie session
// the request authenticated with ("" for Bearer), carried in the context so the
// logout handler can revoke exactly that session.
func (a *AuthMiddleware) serveAuthenticated(
	w http.ResponseWriter, r *http.Request, next http.Handler, reqLog *zerolog.Logger,
	user *repository.User, sessionToken string, isBootstrapAdmin bool,
) {
	// Enrich the request-scoped logger in place: every downstream line and
	// the access-log line now carry the resolved caller.
	reqLog.UpdateContext(func(c zerolog.Context) zerolog.Context {
		return c.Int64("user_id", user.ID).Str("user", user.Email)
	})

	// Deactivated users keep their data and stay linked, but cannot enter.
	if !user.IsActive {
		reqLog.Warn().Msg("login refused: account deactivated")
		http.Error(w, "account deactivated", http.StatusForbidden)
		return
	}

	// Auto-provision: first-time users (no roles yet) default to staff.
	if err := a.repo.GrantDefaultRoleIfNone(r.Context(), user.ID, model.RoleStaff); err != nil {
		reqLog.Warn().Err(err).Msg("grant default staff role")
	}

	// Bootstrap admin: idempotently ensure the configured identity is an admin.
	// Applied on the cookie path too, so a 60-day session never becomes a
	// 60-day window in which that account can stay demoted.
	if isBootstrapAdmin {
		if err := a.repo.EnsureUserHasRole(r.Context(), user.ID, model.RoleAdmin); err != nil {
			reqLog.Warn().Err(err).Msg("ensure bootstrap admin role")
		}
	}

	roles, err := a.repo.GetUserRoles(r.Context(), user.ID)
	if err != nil {
		reqLog.Error().Err(err).Msg("get user roles")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	ctx := context.WithValue(r.Context(), CtxUser, user)
	ctx = context.WithValue(ctx, CtxRoles, roles)
	if sessionToken != "" {
		ctx = ContextWithSessionToken(ctx, sessionToken)
	}
	next.ServeHTTP(w, r.WithContext(ctx))
}

func extractBearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return h[7:]
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func UserFromCtx(ctx context.Context) *repository.User {
	u, _ := ctx.Value(CtxUser).(*repository.User)
	return u
}

func RolesFromCtx(ctx context.Context) []string {
	roles, _ := ctx.Value(CtxRoles).([]string)
	return roles
}

// HasRole reports whether the caller holds the given role (admin always passes).
func HasRole(ctx context.Context, role string) bool {
	for _, r := range RolesFromCtx(ctx) {
		if r == role || r == model.RoleAdmin {
			return true
		}
	}
	return false
}

// HasProcurementAccess reports whether the caller may perform procurement actions
// (quotations, contracts, approvals, PR rejection). Any procurement,
// procurement_admin or admin user qualifies.
func HasProcurementAccess(ctx context.Context) bool {
	return HasRole(ctx, model.RoleProcurement) ||
		HasRole(ctx, model.RoleProcurementAdmin) ||
		HasRole(ctx, model.RoleAdmin)
}

// HasVendorAdmin reports whether the caller may manage the vendor master from
// the dedicated vendor management page (edit, deactivate). Only procurement_admin
// and admin qualify; plain procurement users keep read + inline-create access.
func HasVendorAdmin(ctx context.Context) bool {
	return HasRole(ctx, model.RoleProcurementAdmin) || HasRole(ctx, model.RoleAdmin)
}

// HasTeamAdmin reports whether the caller may manage teams — add/remove members
// (which grants/revokes the team's member role) and edit the team email. Only
// procurement_admin and admin qualify. Reading teams is open to any authenticated user.
func HasTeamAdmin(ctx context.Context) bool {
	return HasRole(ctx, model.RoleProcurementAdmin) || HasRole(ctx, model.RoleAdmin)
}

// HasBusinessUnitAdmin reports whether the caller may manage master data —
// business units (create, edit, deactivate) and the requisition-form config
// options — from their dedicated admin pages. Only procurement_admin and admin
// qualify; the active-business-unit lookup used to populate a purchase request's
// dropdown is open to any authenticated user.
func HasBusinessUnitAdmin(ctx context.Context) bool {
	return HasRole(ctx, model.RoleProcurementAdmin) || HasRole(ctx, model.RoleAdmin)
}
