package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewSessionTokenIsRandomAndHashed(t *testing.T) {
	a, aHash, err := NewSessionToken()
	if err != nil {
		t.Fatalf("new token: %v", err)
	}
	b, _, err := NewSessionToken()
	if err != nil {
		t.Fatalf("new token: %v", err)
	}
	if a == b {
		t.Fatal("two tokens came out identical")
	}
	if len(a) < 40 {
		t.Fatalf("token %q is shorter than 256 bits of base64", a)
	}
	// The cookie value must never be recoverable from what we store.
	if aHash == a {
		t.Fatal("stored hash equals the token itself")
	}
	if got := HashSessionToken(a); got != aHash {
		t.Fatalf("hash not stable: %q != %q", got, aHash)
	}
}

// The __Host- prefix is only legal on a Secure cookie, so the name must track
// the Secure setting — dev over plain HTTP uses the unprefixed name.
func TestCookieNameTracksSecure(t *testing.T) {
	if got := (SessionConfig{Secure: true}).CookieName(); got != SessionCookieName {
		t.Fatalf("secure cookie name = %q, want %q", got, SessionCookieName)
	}
	if got := (SessionConfig{Secure: false}).CookieName(); got != SessionCookieNameDev {
		t.Fatalf("dev cookie name = %q, want %q", got, SessionCookieNameDev)
	}
}

func TestSetAndClearSessionCookie(t *testing.T) {
	cfg := SessionConfig{Enabled: true, TTL: 60 * 24 * time.Hour, Secure: true, SameSite: http.SameSiteNoneMode}
	rec := httptest.NewRecorder()
	SetSessionCookie(rec, cfg, "tok", time.Now().Add(cfg.TTL))

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("got %d cookies, want 1", len(cookies))
	}
	c := cookies[0]
	if c.Name != SessionCookieName || c.Value != "tok" {
		t.Fatalf("cookie = %s=%s", c.Name, c.Value)
	}
	if !c.HttpOnly || !c.Secure || c.Path != "/" {
		t.Fatalf("cookie attributes: httponly=%v secure=%v path=%q", c.HttpOnly, c.Secure, c.Path)
	}
	if c.MaxAge < int((59 * 24 * time.Hour).Seconds()) {
		t.Fatalf("max-age %ds is not ~60 days", c.MaxAge)
	}
	// SameSite=None makes this a third-party cookie unless it is partitioned,
	// and browsers that block those would drop it silently.
	if !c.Partitioned {
		t.Fatal("SameSite=None cookie is not Partitioned")
	}

	// The clearing cookie must repeat the attributes, or the browser keeps the
	// original and "sign out" leaves a working session behind.
	rec = httptest.NewRecorder()
	ClearSessionCookie(rec, cfg)
	c = rec.Result().Cookies()[0]
	if c.Name != SessionCookieName || c.Value != "" || c.MaxAge >= 0 {
		t.Fatalf("clearing cookie = %s=%q max-age=%d", c.Name, c.Value, c.MaxAge)
	}
}

func TestCheckCSRFOrigin(t *testing.T) {
	allowed := []string{"https://app.example.com"}

	newReq := func(method, origin string) *http.Request {
		r := httptest.NewRequest(method, "https://api.example.com/api/v1/purchase-requests", nil)
		r.Host = "api.example.com"
		r.Header.Set("X-Forwarded-Proto", "https")
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		return r
	}

	cases := []struct {
		name   string
		req    *http.Request
		accept bool
	}{
		// Reads are never CSRF-able, and the cookie is the only credential a
		// browser sends automatically — a GET needs no origin proof.
		{"read from anywhere", newReq(http.MethodGet, "https://evil.example.com"), true},
		{"write from the app origin", newReq(http.MethodPost, "https://app.example.com"), true},
		{"write from the API's own origin", newReq(http.MethodPost, "https://api.example.com"), true},
		// Non-browser clients omit Origin and authenticate with a Bearer token.
		{"write with no origin header", newReq(http.MethodPost, ""), true},
		{"write from another site", newReq(http.MethodPost, "https://evil.example.com"), false},
		{"delete from another site", newReq(http.MethodDelete, "https://evil.example.com"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CheckCSRFOrigin(tc.req, allowed); got != tc.accept {
				t.Fatalf("CheckCSRFOrigin = %v, want %v", got, tc.accept)
			}
		})
	}
}

func TestClientIPPrefersForwardedHeader(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.1:54321"
	if got := ClientIP(r); got != "10.0.0.1" {
		t.Fatalf("ClientIP = %q, want the RemoteAddr host", got)
	}
	r.Header.Set("X-Forwarded-For", "203.0.113.7, 10.0.0.1")
	if got := ClientIP(r); got != "203.0.113.7" {
		t.Fatalf("ClientIP = %q, want the first X-Forwarded-For entry", got)
	}
}
