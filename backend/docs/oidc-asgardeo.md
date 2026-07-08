# OIDC with Asgardeo — the `x509: negative serial number` gotcha

## Symptom

Every API call fails auth. The backend logs:

```
token verification failed: failed to verify signature: fetching keys
oidc: failed to decode keys: … go-jose/go-jose: failed to unmarshal x5c field:
x509: negative serial number
```

The OIDC config is correct (issuer/client_id/discovery all match the token), the
token is valid (signature/iss/aud verify against the live JWKS), and the JWKS is
reachable — yet verification fails.

## Cause

Asgardeo's JWKS (`https://api.asgardeo.io/t/<tenant>/oauth2/jwks`) includes an
`x5c` X.509 certificate chain alongside the RSA public key. That self-signed
cert (`CN=wso2, O=None`) has a **negative serial number**. Go 1.23+ rejects
certificates with negative serial numbers by default (RFC 5280 §4.1.2.2), and
go-jose — used by `go-oidc` — parses the `x5c` field while decoding the key set.
So key decoding fails before the signature is ever checked. The actual signing
key (`n`/`e` in the JWK) is fine; only the attached cert is non-compliant.

## Fix

Restore the pre-1.23 behavior of tolerating negative serial numbers via a
`//go:debug` directive in the main package (`cmd/server/main.go`):

```go
//go:debug x509negativeserial=1
package main
```

This bakes `DefaultGODEBUG=x509negativeserial=1` into the binary (verify with
`go version -m <binary> | grep godebug`) — no env var to set in Choreo. Verified
against Go 1.25.8 and the actual Asgardeo JWKS: go-jose decodes the key set and
tokens verify. Equivalent runtime alternative: set `GODEBUG=x509negativeserial=1`
as an environment variable on the component.

This affects **only** cert parsing for the JWKS; it does not weaken token
signature verification, which still uses the RSA key and checks iss/aud/exp/sig.
