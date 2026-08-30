package middleware

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

// Accepting a second audience is what lets a second front end — its own Asgardeo
// application, and so its own `aud` — authenticate here (docs/plans/21, Decision
// 4). The risk it introduces is accepting one audience too many: every app in the
// Asgardeo organisation shares this issuer and signing key, so audience is the
// ONLY thing separating "our other front end" from "some unrelated app in the
// same tenant". These cases pin that boundary.

const (
	primaryClientID = "purchasing-spa-client"
	portalClientID  = "one-wso2-spa-client"
	foreignClientID = "some-other-app-in-the-tenant"
)

// newAudienceTestAuth builds the middleware with real verifiers over one signing
// key, standing in for the IdP's JWKS. oidc.NewVerifier (rather than
// provider.Verifier) keeps this offline — no discovery round trip.
func newAudienceTestAuth(t *testing.T, clientIDs ...string) (*AuthMiddleware, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	ks := &staticKeySet{key: &key.PublicKey}
	verifiers := make([]*oidc.IDTokenVerifier, 0, len(clientIDs))
	for _, id := range clientIDs {
		verifiers = append(verifiers, oidc.NewVerifier(testIssuer, ks, &oidc.Config{ClientID: id}))
	}
	return &AuthMiddleware{
		verifiers:         verifiers,
		acceptedClientIDs: clientIDs,
		issuer:            testIssuer,
		clientID:          clientIDs[0],
		keySet:            ks,
	}, key
}

// idTokenClaims is a well-formed ID token for one audience.
func idTokenClaims(aud string) map[string]any {
	return map[string]any{
		"iss":   testIssuer,
		"aud":   aud,
		"sub":   "idp-user-123",
		"email": "someone@example.com",
		"iat":   time.Now().Unix(),
		"exp":   time.Now().Add(time.Hour).Unix(),
	}
}

func TestVerifyIDTokenAcceptsPrimaryAudience(t *testing.T) {
	am, key := newAudienceTestAuth(t, primaryClientID, portalClientID)

	tok, err := am.verifyIDToken(context.Background(), signClaims(t, key, idTokenClaims(primaryClientID)))
	if err != nil {
		t.Fatalf("primary audience rejected: %v", err)
	}
	if tok.Subject != "idp-user-123" {
		t.Errorf("sub = %q, want idp-user-123", tok.Subject)
	}
}

// The whole point of the change: the portal's own audience authenticates.
func TestVerifyIDTokenAcceptsAdditionalAudience(t *testing.T) {
	am, key := newAudienceTestAuth(t, primaryClientID, portalClientID)

	if _, err := am.verifyIDToken(context.Background(), signClaims(t, key, idTokenClaims(portalClientID))); err != nil {
		t.Fatalf("additional audience rejected: %v", err)
	}
}

// The boundary. A token signed by the SAME key, from the SAME issuer, for an
// application nobody configured, must not authenticate — otherwise any app in
// the Asgardeo organisation could mint credentials for this one.
func TestVerifyIDTokenRejectsForeignAudience(t *testing.T) {
	am, key := newAudienceTestAuth(t, primaryClientID, portalClientID)

	_, err := am.verifyIDToken(context.Background(), signClaims(t, key, idTokenClaims(foreignClientID)))
	if err == nil {
		t.Fatal("a token for an unconfigured audience was accepted")
	}
	if !strings.Contains(err.Error(), "audience") && !strings.Contains(err.Error(), "aud") {
		t.Errorf("error should name the audience problem, got: %v", err)
	}
}

// Configuring no additional audiences must behave exactly as the single-audience
// code did — this is the rollback position.
func TestVerifyIDTokenSingleAudienceUnchanged(t *testing.T) {
	am, key := newAudienceTestAuth(t, primaryClientID)

	if _, err := am.verifyIDToken(context.Background(), signClaims(t, key, idTokenClaims(primaryClientID))); err != nil {
		t.Fatalf("own audience rejected: %v", err)
	}
	if _, err := am.verifyIDToken(context.Background(), signClaims(t, key, idTokenClaims(portalClientID))); err == nil {
		t.Fatal("an unconfigured audience was accepted with no additional_client_ids set")
	}
}

// Expiry is still enforced on the additional audience — the extra verifier is a
// full ID-token verifier, not an audience waiver.
func TestVerifyIDTokenRejectsExpiredAdditionalAudience(t *testing.T) {
	am, key := newAudienceTestAuth(t, primaryClientID, portalClientID)

	claims := idTokenClaims(portalClientID)
	claims["iat"] = time.Now().Add(-2 * time.Hour).Unix()
	claims["exp"] = time.Now().Add(-time.Hour).Unix()

	if _, err := am.verifyIDToken(context.Background(), signClaims(t, key, claims)); err == nil {
		t.Fatal("an expired token was accepted for the additional audience")
	}
}

// A token signed by a key the IdP does not publish is rejected regardless of how
// many audiences are configured — the loop must not weaken signature checking.
func TestVerifyIDTokenRejectsForeignSigningKey(t *testing.T) {
	am, _ := newAudienceTestAuth(t, primaryClientID, portalClientID)
	attacker, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	if _, err := am.verifyIDToken(context.Background(), signClaims(t, attacker, idTokenClaims(portalClientID))); err == nil {
		t.Fatal("a token signed by a foreign key was accepted")
	}
}

// Back-channel logout must NOT inherit the widened audience: its job is to revoke
// this app's own cookie sessions, which only this app's own front end holds. A
// logout token from the portal's client is therefore not a valid instruction here.
func TestLogoutTokenAudienceStaysBoundToOwnClient(t *testing.T) {
	am, key := newAudienceTestAuth(t, primaryClientID, portalClientID)

	claims := validClaims()
	claims["aud"] = portalClientID
	if _, err := am.VerifyLogoutToken(context.Background(), signClaims(t, key, claims)); err == nil {
		t.Fatal("a logout token for an additional audience was accepted")
	}

	claims = validClaims()
	claims["aud"] = primaryClientID
	if _, err := am.VerifyLogoutToken(context.Background(), signClaims(t, key, claims)); err != nil {
		t.Fatalf("a logout token for our own client was rejected: %v", err)
	}
}

func TestDedupeExcluding(t *testing.T) {
	got := dedupeExcluding([]string{" b ", "a", "b", "", "a", "c"}, "a")
	want := []string{"b", "c"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

// The REAL token shape, captured from One WSO2 on 2026-08-30. Asgardeo mints
// Choreo-bound access tokens with `aud` as an ARRAY, carrying the client id
// alongside a Choreo environment marker:
//
//	"aud": ["YW2Q2Kb...", "choreo:deployment:sandbox"]
//
// This is the case that actually has to work in production, and it is not the
// single-string case every other test here covers. Asserted rather than assumed:
// go-oidc tests membership of the array, so the extra entry is ignored.
func TestVerifyIDTokenAcceptsArrayAudienceContainingConfiguredClient(t *testing.T) {
	am, key := newAudienceTestAuth(t, primaryClientID, portalClientID)

	claims := idTokenClaims(portalClientID)
	claims["aud"] = []any{portalClientID, "choreo:deployment:sandbox"}

	if _, err := am.verifyIDToken(context.Background(), signClaims(t, key, claims)); err != nil {
		t.Fatalf("array audience containing the configured client was rejected: %v", err)
	}
}

// The other half of that, and the one that would bite silently. An array
// audience must still be REJECTED when none of its entries is configured —
// membership, not "the array is non-empty". Without this, a token minted for
// any other Choreo app in the organisation would authenticate here purely
// because it shares the `choreo:deployment:*` marker.
func TestVerifyIDTokenRejectsArrayAudienceWithoutConfiguredClient(t *testing.T) {
	am, key := newAudienceTestAuth(t, primaryClientID, portalClientID)

	claims := idTokenClaims(foreignClientID)
	claims["aud"] = []any{foreignClientID, "choreo:deployment:sandbox"}

	if _, err := am.verifyIDToken(context.Background(), signClaims(t, key, claims)); err == nil {
		t.Fatal("an array audience with no configured entry was accepted")
	}
}
