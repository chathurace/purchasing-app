package middleware

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
	"time"
)

// SessionCookieName is the cookie carrying the backend-issued session token.
// The __Host- prefix is a browser-enforced guarantee: the cookie is accepted
// only when it is Secure, has Path=/, and carries NO Domain attribute — which
// pins it to the exact origin that set it, so a sibling subdomain cannot
// overwrite it (session fixation). Browsers reject the prefix over plain HTTP,
// so local dev (http://localhost) uses the unprefixed name; see CookieName.
const SessionCookieName = "__Host-purchasing_session"

// SessionCookieNameDev is the plain-HTTP fallback name used when Secure is off.
const SessionCookieNameDev = "purchasing_session"

// SessionConfig holds the backend-issued session settings (from the `session:`
// block in config.yaml — see config.Config).
type SessionConfig struct {
	// Enabled turns cookie sessions on. When false the backend behaves exactly
	// as it did before them (Bearer tokens only), which is the rollback switch
	// if anything goes wrong in production.
	Enabled bool
	// TTL is the *idle* lifetime of a session: every authenticated request
	// slides it forward (see Repository.TouchUserSession).
	TTL time.Duration
	// Secure sets the cookie's Secure attribute. Only ever false for local dev
	// over http://localhost.
	Secure bool
	// SameSite is Lax when the API is same-site with the SPA (localhost:5173 →
	// localhost:8081 in dev, or an nginx-proxied /api in production) and None
	// when the SPA talks to the backend cross-site, which additionally requires
	// Secure and is blocked by browsers that reject third-party cookies.
	SameSite http.SameSite
	// AllowedOrigins mirrors cors.allowed_origins and backs the CSRF origin
	// check on cookie-authenticated writes.
	AllowedOrigins []string
}

// CookieName returns the cookie name matching the Secure setting — the __Host-
// prefix is only legal on a Secure cookie.
func (c SessionConfig) CookieName() string {
	if c.Secure {
		return SessionCookieName
	}
	return SessionCookieNameDev
}

// NewSessionToken returns a fresh 256-bit session token (URL-safe base64) and
// its SHA-256 hash. Only the hash is ever persisted, so read access to the
// database — or to a backup — does not yield a usable session.
func NewSessionToken() (token, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	token = base64.RawURLEncoding.EncodeToString(buf)
	return token, HashSessionToken(token), nil
}

// HashSessionToken returns the storage form of a session token.
func HashSessionToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// SetSessionCookie writes the session cookie for the given token.
//
// In SameSite=None mode the cookie is also marked Partitioned (CHIPS): the SPA
// and the API are on different sites there, so without partitioning the cookie
// is an ordinary third-party cookie and browsers that restrict those silently
// drop it — which looks exactly like "logged out again". Partitioned keys the
// cookie to the SPA's top-level site, which is precisely the scope wanted.
// Safari blocks it either way; that is the case a same-site (proxied) API
// deployment exists to solve.
func SetSessionCookie(w http.ResponseWriter, cfg SessionConfig, token string, expiresAt time.Time) {
	http.SetCookie(w, sessionCookie(cfg, token, expiresAt, int(time.Until(expiresAt).Seconds())))
}

// ClearSessionCookie expires the session cookie. The attributes must match the
// ones used when setting it or the browser keeps the original cookie.
func ClearSessionCookie(w http.ResponseWriter, cfg SessionConfig) {
	http.SetCookie(w, sessionCookie(cfg, "", time.Unix(0, 0), -1))
}

func sessionCookie(cfg SessionConfig, token string, expires time.Time, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:        cfg.CookieName(),
		Value:       token,
		Path:        "/",
		Expires:     expires,
		MaxAge:      maxAge,
		HttpOnly:    true, // never readable by JavaScript
		Secure:      cfg.Secure,
		SameSite:    cfg.SameSite,
		Partitioned: cfg.SameSite == http.SameSiteNoneMode,
	}
}

// SessionTokenFromRequest returns the session token presented by the client, if
// any.
func SessionTokenFromRequest(r *http.Request, cfg SessionConfig) string {
	c, err := r.Cookie(cfg.CookieName())
	if err != nil || c.Value == "" {
		return ""
	}
	return c.Value
}

// CheckCSRFOrigin guards cookie-authenticated state-changing requests.
//
// Cookies are attached by the browser automatically, so unlike a Bearer token
// they are vulnerable to cross-site request forgery. Two layers protect the
// write paths:
//
//  1. SameSite=Lax on the cookie — the browser withholds it from cross-site
//     POST/PUT/DELETE entirely. This is the primary defence and needs no server
//     cooperation.
//  2. This check — the request's Origin must be one of the configured app
//     origins (or the API's own origin). It is what still holds when SameSite
//     has to be None (cross-site API deployment), and it defends against a
//     browser bug or a future SameSite downgrade.
//
// A missing Origin header is allowed: non-browser clients (curl, scripts) omit
// it and authenticate with a Bearer token anyway. Browsers always send Origin
// on the methods checked here, so an absent header is never a forged
// cross-site write.
func CheckCSRFOrigin(r *http.Request, allowed []string) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	// Same-origin requests (a proxied /api deployment) present the API's own
	// origin, which is not in the configured SPA origin list.
	if sameOrigin(r, origin) {
		return true
	}
	for _, a := range allowed {
		if a == "*" || strings.EqualFold(a, origin) {
			return true
		}
	}
	return false
}

// sameOrigin reports whether origin names the host this request was sent to.
func sameOrigin(r *http.Request, origin string) bool {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	// Behind the Choreo/nginx proxy the original scheme survives only in
	// X-Forwarded-Proto; the connection to the backend itself is plain HTTP.
	if fwd := r.Header.Get("X-Forwarded-Proto"); fwd != "" {
		scheme = fwd
	}
	return strings.EqualFold(origin, scheme+"://"+r.Host)
}

// ClientIP returns the caller's IP, honouring X-Forwarded-For / X-Real-IP set
// by the proxy in front of the app. Recorded on a session row for forensics
// only — never used to authenticate.
func ClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i > 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if xr := r.Header.Get("X-Real-Ip"); xr != "" {
		return strings.TrimSpace(xr)
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// ctxKeySessionToken carries the presented session token so the logout handler
// can revoke exactly the session it was called with.
type ctxKeySessionToken struct{}

// ContextWithSessionToken returns a copy of ctx carrying the session token the
// request authenticated with.
func ContextWithSessionToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, ctxKeySessionToken{}, token)
}

// SessionTokenFromContext returns the session token the request authenticated
// with, or "" for Bearer-authenticated requests.
func SessionTokenFromContext(ctx context.Context) string {
	t, _ := ctx.Value(ctxKeySessionToken{}).(string)
	return t
}
