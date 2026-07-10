package middleware

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
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
	log                 zerolog.Logger
}

// AuthConfig carries the OIDC settings (sourced from config.yaml).
type AuthConfig struct {
	Issuer             string
	DiscoveryURL       string
	ClientID           string
	InsecureSkipVerify bool
	BootstrapAdminEmail string
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
	// Log the EFFECTIVE OIDC config the running server loaded, so "is the deploy
	// actually using my config.yaml?" is answerable from the boot logs alone.
	log.Info().
		Str("issuer", cfg.Issuer).
		Str("discovery_url", discoveryURL).
		Str("client_id", cfg.ClientID).
		Bool("insecure_skip_verify", cfg.InsecureSkipVerify).
		Msg("OIDC initialized")
	if cfg.BootstrapAdminEmail != "" {
		log.Info().Str("email", cfg.BootstrapAdminEmail).Msg("bootstrap admin configured")
	}
	return &AuthMiddleware{
		verifier:            verifier,
		repo:                repo,
		bootstrapAdminEmail: cfg.BootstrapAdminEmail,
		log:                 log,
	}, nil
}

func (a *AuthMiddleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Request-scoped logger installed by middleware.RequestLogger — carries
		// the request_id, and (once we resolve the caller below) the user.
		reqLog := zerolog.Ctx(r.Context())

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
		// Match across the claims that may carry it (email, username, or sub).
		if a.bootstrapAdminEmail != "" {
			if email == a.bootstrapAdminEmail ||
				claims.PreferredUsername == a.bootstrapAdminEmail ||
				claims.Username == a.bootstrapAdminEmail ||
				claims.Sub == a.bootstrapAdminEmail {
				if err := a.repo.EnsureUserHasRole(r.Context(), user.ID, model.RoleAdmin); err != nil {
					reqLog.Warn().Err(err).Msg("ensure bootstrap admin role")
				}
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
		next.ServeHTTP(w, r.WithContext(ctx))
	})
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

// HasFinanceAccess reports whether the caller may perform procurement actions
// (quotations, contracts, approvals, PR rejection). Any finance,
// finance_admin or admin user qualifies.
func HasFinanceAccess(ctx context.Context) bool {
	return HasRole(ctx, model.RoleFinance) ||
		HasRole(ctx, model.RoleFinanceAdmin) ||
		HasRole(ctx, model.RoleAdmin)
}

// HasVendorAdmin reports whether the caller may manage the vendor master from
// the dedicated vendor management page (edit, deactivate). Only finance_admin
// and admin qualify; plain finance users keep read + inline-create access.
func HasVendorAdmin(ctx context.Context) bool {
	return HasRole(ctx, model.RoleFinanceAdmin) || HasRole(ctx, model.RoleAdmin)
}

// HasCostCenterAdmin reports whether the caller may manage the cost-center
// master from the dedicated cost centers page (create, edit, deactivate). Only
// finance_admin and admin qualify; the active-cost-center lookup used to
// populate a purchase request's dropdown is open to any authenticated user.
func HasCostCenterAdmin(ctx context.Context) bool {
	return HasRole(ctx, model.RoleFinanceAdmin) || HasRole(ctx, model.RoleAdmin)
}
