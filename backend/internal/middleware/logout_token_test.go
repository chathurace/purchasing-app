package middleware

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
)

// The logout token IS the credential on the back-channel logout route — there is
// no cookie or Bearer token — so these cases are the whole authorization check.
// Each one mints a real RSA-signed JWT and runs it through VerifyLogoutToken
// against a key set holding the matching public key.

const (
	testIssuer   = "https://idp.example.com/oauth2/token"
	testClientID = "test-client-id"
)

// staticKeySet implements oidc.KeySet over one public key, standing in for the
// IdP's JWKS endpoint.
type staticKeySet struct {
	key *rsa.PublicKey
}

func (s *staticKeySet) VerifySignature(_ context.Context, jwt string) ([]byte, error) {
	sig, err := jose.ParseSigned(jwt, []jose.SignatureAlgorithm{jose.RS256})
	if err != nil {
		return nil, err
	}
	return sig.Verify(s.key)
}

// signClaims returns claims as a signed JWT, and the middleware wired to verify
// it. Deliberately signs whatever it is given — including invalid tokens.
func signClaims(t *testing.T, key *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, nil)
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	obj, err := signer.Sign(payload)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	raw, err := obj.CompactSerialize()
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	return raw
}

func newLogoutTestAuth(t *testing.T) (*AuthMiddleware, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return &AuthMiddleware{
		keySet:   &staticKeySet{key: &key.PublicKey},
		issuer:   testIssuer,
		clientID: testClientID,
	}, key
}

// validClaims is a well-formed logout token naming both a session and a subject.
func validClaims() map[string]any {
	return map[string]any{
		"iss": testIssuer,
		"aud": testClientID,
		"sub": "idp-user-123",
		"sid": "idp-session-abc",
		"iat": time.Now().Unix(),
		"jti": "jti-1",
		"events": map[string]any{
			backchannelLogoutEvent: map[string]any{},
		},
	}
}

func TestVerifyLogoutTokenAccepts(t *testing.T) {
	am, key := newLogoutTestAuth(t)

	got, err := am.VerifyLogoutToken(context.Background(), signClaims(t, key, validClaims()))
	if err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}
	if got.SessionID != "idp-session-abc" || got.Subject != "idp-user-123" {
		t.Fatalf("got %+v, want sid/sub from the token", got)
	}

	// sid-only and sub-only are both legal (spec §2.4 requires at least one).
	for name, drop := range map[string]string{"sid only": "sub", "sub only": "sid"} {
		t.Run(name, func(t *testing.T) {
			c := validClaims()
			delete(c, drop)
			if _, err := am.VerifyLogoutToken(context.Background(), signClaims(t, key, c)); err != nil {
				t.Fatalf("%s rejected: %v", name, err)
			}
		})
	}

	// aud may be an array rather than a string.
	t.Run("aud as array", func(t *testing.T) {
		c := validClaims()
		c["aud"] = []string{"someone-else", testClientID}
		if _, err := am.VerifyLogoutToken(context.Background(), signClaims(t, key, c)); err != nil {
			t.Fatalf("array aud rejected: %v", err)
		}
	})
}

func TestVerifyLogoutTokenRejects(t *testing.T) {
	am, key := newLogoutTestAuth(t)

	cases := map[string]func(c map[string]any){
		// Another tenant app's token must not sign our users out.
		"wrong audience": func(c map[string]any) { c["aud"] = "another-client" },
		"wrong issuer":   func(c map[string]any) { c["iss"] = "https://evil.example.com" },
		// Without the events claim this is just some JWT — most importantly, an
		// ID token replayed as a logout instruction.
		"missing events": func(c map[string]any) { delete(c, "events") },
		"wrong event": func(c map[string]any) {
			c["events"] = map[string]any{"http://schemas.openid.net/event/other": map[string]any{}}
		},
		// A nonce marks it as an ID token (spec §2.4 forbids it here).
		"nonce present":  func(c map[string]any) { c["nonce"] = "n-123" },
		"missing iat":    func(c map[string]any) { delete(c, "iat") },
		"stale iat":      func(c map[string]any) { c["iat"] = time.Now().Add(-time.Hour).Unix() },
		"future iat":     func(c map[string]any) { c["iat"] = time.Now().Add(time.Hour).Unix() },
		"expired":        func(c map[string]any) { c["exp"] = time.Now().Add(-time.Minute).Unix() },
		"no sid nor sub": func(c map[string]any) { delete(c, "sid"); delete(c, "sub") },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			c := validClaims()
			mutate(c)
			if _, err := am.VerifyLogoutToken(context.Background(), signClaims(t, key, c)); err == nil {
				t.Fatalf("%s was accepted", name)
			}
		})
	}

	t.Run("signed by another key", func(t *testing.T) {
		other, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("generate key: %v", err)
		}
		if _, err := am.VerifyLogoutToken(context.Background(), signClaims(t, other, validClaims())); err == nil {
			t.Fatal("a token signed by an unknown key was accepted")
		}
	})

	t.Run("unsigned token", func(t *testing.T) {
		// alg=none, the classic JWT bypass.
		header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
		payload, _ := json.Marshal(validClaims())
		raw := header + "." + base64.RawURLEncoding.EncodeToString(payload) + "."
		if _, err := am.VerifyLogoutToken(context.Background(), raw); err == nil {
			t.Fatal("an unsigned token was accepted")
		}
	})

	t.Run("garbage", func(t *testing.T) {
		if _, err := am.VerifyLogoutToken(context.Background(), "not-a-jwt"); err == nil {
			t.Fatal("a non-JWT was accepted")
		}
	})

	// With no key set (jwks_uri unresolved at startup) nothing may verify.
	t.Run("no key set", func(t *testing.T) {
		bare := &AuthMiddleware{issuer: testIssuer, clientID: testClientID}
		if bare.BackchannelLogoutReady() {
			t.Fatal("BackchannelLogoutReady is true without a key set")
		}
		if _, err := bare.VerifyLogoutToken(context.Background(), signClaims(t, key, validClaims())); err == nil ||
			!strings.Contains(err.Error(), "no key set") {
			t.Fatalf("got %v, want a no-key-set error", err)
		}
	})
}
