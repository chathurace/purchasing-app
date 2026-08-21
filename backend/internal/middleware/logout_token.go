package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

// OIDC Back-Channel Logout (docs/sessions.md).
//
// When the IdP ends a session — the user signs out there, or an admin terminates
// it — it POSTs a signed *logout token* to the app. This file verifies that
// token; the handler decides what to revoke. It is the only way the app learns
// about an IdP-side sign-out, because after a session cookie is minted the IdP is
// never consulted again.
//
// The logout token is deliberately NOT verified with the ID-token verifier: an
// ID token and a logout token are different artifacts with different required
// claims (a logout token carries `events`, must NOT carry `nonce`, and its `exp`
// is optional), so reusing that verifier would either reject valid tokens or
// accept an ID token as a logout instruction. See OpenID Connect Back-Channel
// Logout 1.0 §2.4 for the validation rules implemented below.

// backchannelLogoutEvent is the required member of the `events` claim.
const backchannelLogoutEvent = "http://schemas.openid.net/event/backchannel-logout"

// logoutTokenMaxAge bounds replay: a captured token stays usable only this long.
// The spec suggests a short window and recommends `jti` tracking on top; the
// revocation this drives is idempotent, so a replay inside the window merely
// re-revokes an already-revoked session.
const logoutTokenMaxAge = 5 * time.Minute

// ErrNoLogoutSubject is returned when a token is otherwise valid but names
// neither a session nor a subject, so there is nothing to act on.
var ErrNoLogoutSubject = errors.New("logout token has neither sid nor sub")

// LogoutToken is the verified content of a back-channel logout token.
type LogoutToken struct {
	// Subject is the IdP's user id (`sub`), empty when only a session is named.
	Subject string
	// SessionID is the IdP session id (`sid`), empty when only a subject is
	// named. Preferred when present: it identifies one browser rather than
	// everything that person is signed in on.
	SessionID string
}

// logoutTokenClaims is the wire shape of the token payload.
type logoutTokenClaims struct {
	Issuer   string          `json:"iss"`
	Audience audienceClaim   `json:"aud"`
	Subject  string          `json:"sub"`
	SID      string          `json:"sid"`
	IssuedAt int64           `json:"iat"`
	Expiry   int64           `json:"exp"`
	JTI      string          `json:"jti"`
	Events   map[string]any  `json:"events"`
	Nonce    string          `json:"nonce"`
	Extra    json.RawMessage `json:"-"`
}

// audienceClaim accepts `aud` in either of its legal JSON forms — a string or an
// array of strings.
type audienceClaim []string

func (a *audienceClaim) UnmarshalJSON(b []byte) error {
	var one string
	if err := json.Unmarshal(b, &one); err == nil {
		*a = []string{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err != nil {
		return err
	}
	*a = many
	return nil
}

func (a audienceClaim) contains(v string) bool {
	for _, s := range a {
		if s == v {
			return true
		}
	}
	return false
}

// VerifyLogoutToken checks a back-channel logout token's signature and claims,
// returning what it says should be logged out. Every failure is a 400 to the
// caller: an unverifiable token is indistinguishable from an attacker asking us
// to sign somebody out.
func (a *AuthMiddleware) VerifyLogoutToken(ctx context.Context, raw string) (*LogoutToken, error) {
	if a.keySet == nil {
		return nil, errors.New("logout token verification unavailable: no key set")
	}
	// Signature first — nothing in the payload is trustworthy until this passes.
	payload, err := a.keySet.VerifySignature(ctx, raw)
	if err != nil {
		return nil, fmt.Errorf("signature: %w", err)
	}
	var c logoutTokenClaims
	if err := json.Unmarshal(payload, &c); err != nil {
		return nil, fmt.Errorf("malformed claims: %w", err)
	}

	if c.Issuer != a.issuer {
		return nil, fmt.Errorf("issuer %q is not %q", c.Issuer, a.issuer)
	}
	// Audience binding: the token must have been minted for this client, not for
	// another app in the same tenant.
	if !c.Audience.contains(a.clientID) {
		return nil, fmt.Errorf("audience %v does not contain this client", []string(c.Audience))
	}
	// The events claim is what distinguishes a logout token from an ID token.
	if _, ok := c.Events[backchannelLogoutEvent]; !ok {
		return nil, errors.New("events claim does not contain the backchannel-logout event")
	}
	// A logout token must not carry a nonce (spec §2.4); one present means an ID
	// token is being replayed at us as a logout instruction.
	if c.Nonce != "" {
		return nil, errors.New("logout token must not contain a nonce")
	}
	now := time.Now()
	if c.IssuedAt == 0 {
		return nil, errors.New("missing iat")
	}
	issued := time.Unix(c.IssuedAt, 0)
	// Allow a little clock skew in both directions, then bound replay.
	if issued.After(now.Add(time.Minute)) {
		return nil, errors.New("iat is in the future")
	}
	if now.Sub(issued) > logoutTokenMaxAge {
		return nil, fmt.Errorf("token is older than %s", logoutTokenMaxAge)
	}
	// exp is optional here (unlike an ID token), but honour it when present.
	if c.Expiry != 0 && now.After(time.Unix(c.Expiry, 0)) {
		return nil, errors.New("token is expired")
	}
	if c.SID == "" && c.Subject == "" {
		return nil, ErrNoLogoutSubject
	}
	return &LogoutToken{Subject: c.Subject, SessionID: c.SID}, nil
}

// keySetFromProvider pulls the IdP's jwks_uri out of the discovery document and
// builds a caching remote key set from it. Used for logout tokens; ID tokens go
// through provider.Verifier, which keeps its own.
func keySetFromProvider(ctx context.Context, provider *oidc.Provider) (oidc.KeySet, string, error) {
	var meta struct {
		JWKSURL string `json:"jwks_uri"`
	}
	if err := provider.Claims(&meta); err != nil {
		return nil, "", fmt.Errorf("read discovery metadata: %w", err)
	}
	if meta.JWKSURL == "" {
		return nil, "", errors.New("discovery document has no jwks_uri")
	}
	return oidc.NewRemoteKeySet(ctx, meta.JWKSURL), meta.JWKSURL, nil
}
