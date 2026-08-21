# Sessions — 60-day sign-ins (as built)

How long a user stays signed in is now **this app's decision**, not the identity
provider's. Ported from the sibling `finance-apps` implementation.

## Why

Before this, the only session the app had was whatever `oidc-client-ts` held in
the browser. When the ID token expired (or could not be renewed), the SPA
treated it as a logout — so people were signed out roughly daily. The Asgardeo
organisation is managed by another team, so its session and token lifetimes
cannot be raised.

So the app issues its own session (the BFF pattern): the SPA still logs in
through the IdP exactly as before, hands the resulting token to the backend
**once**, and gets back an **HttpOnly cookie** valid for `session.ttl_days`
(**60** by default) — independent of any IdP lifetime.

## How it works

```
browser                          backend
  │  OIDC redirect login (unchanged)
  │  POST /api/v1/auth/session   →  verify Bearer ID token (existing auth path)
  │                                 insert user_sessions row (SHA-256 of token)
  │  ← Set-Cookie: session=…         HttpOnly · Secure · SameSite · Path=/
  │
  │  every later request         →  cookie looked up FIRST, before Authorization
  │  (no IdP contact at all)        expiry slid forward on each request
```

- **`user_sessions`** (migration `052`) stores only the **SHA-256 hash** of a
  256-bit random token, so a DB or backup disclosure yields no usable session.
  Columns: `token_hash`, `user_id`, `created_at`, `last_used_at`, `expires_at`,
  `revoked_at`, `user_agent`, `ip` (the last two are forensics only — both are
  client-controlled, so neither authenticates).
- **Sliding window**: `Repository.TouchUserSession` resolves the token *and*
  pushes `expires_at` to `now + ttl` in one statement, so 60 days means 60 days
  **idle** — an active user is never signed out mid-use, an abandoned session
  still dies on schedule. `GREATEST` means shortening `ttl_days` applies to new
  sessions without extending old ones. `last_used_at` is only rewritten when it
  is over a minute stale (otherwise every request would be a row write).
- **Absolute cap** (`max_days`, default 90): the same statement also refuses a
  row older than the ceiling, measured from `created_at`. Without it the sliding
  window means a session used at least once per idle window lives forever. Set it
  to 0 to disable — the server logs a warning at startup when you do.
- **IdP session id**: `idp_sid` holds the ID token's `sid` claim, captured when
  the cookie is minted (migration `053`). It is what lets a back-channel logout
  end one browser instead of all of them.
- **Cookie name** is `__Host-purchasing_session` when Secure — a
  browser-enforced pin to the exact origin that set it (no `Domain`, `Path=/`,
  Secure), so a sibling subdomain cannot overwrite it. Plain-HTTP dev uses the
  unprefixed `purchasing_session`, since browsers reject the prefix over HTTP.
- **CSRF**: cookies ride along automatically, so cookie-authenticated writes are
  guarded twice — `SameSite` (the browser withholds the cookie from cross-site
  POST/PUT/DELETE) and `CheckCSRFOrigin` (the `Origin` must be a configured app
  origin or the API's own). A missing `Origin` is allowed: non-browser callers
  omit it and use Bearer tokens anyway.
- **Cookie before Bearer** in `middleware.Authenticate`: the IdP token is
  presented exactly once, at `POST /auth/session`. A stale token the browser
  still happens to hold is therefore ignored, not rejected.
- The **bootstrap admin** grant and the **deactivated-account** gate run on the
  cookie path too (`serveAuthenticated`), so a 60-day session is not a 60-day
  window in which either can go stale.
- **Cleanup**: `pruneSessions` (in `cmd/server`) deletes expired rows, and rows
  revoked over a day ago, at boot and then daily. Nothing depends on it for
  correctness — `TouchUserSession` ignores dead rows.

## Ending a session

Not consulting the IdP after mint is the whole point — it is what makes a
session survive token expiry — but it means an IdP-side change is not
automatically felt here. These are all the ways a session ends:

| Trigger | Effect | Where |
| --- | --- | --- |
| User signs out in the app | That session revoked | `DELETE /auth/session` |
| User signs out everywhere | All their sessions revoked | `DELETE /auth/sessions` |
| **Admin ends someone's sessions** | All of that user's sessions revoked; they may sign in again | `DELETE /users/{id}/sessions`, Users page → **End sessions** |
| Admin **deactivates** the user | Cut off on the next request (`is_active` is re-read every request) *and* login blocked | Users page → Deactivate |
| **User signs out at the IdP**, or an IdP admin kills their IdP session | The matching session revoked (by `sid`), or all of them when no `sid` is known | `POST /auth/backchannel-logout` |
| **Account deleted or disabled at the IdP** | All their sessions revoked within one sweep interval (~10 min) | `internal/offboard`, no request involved |
| Idle for `ttl_days` | Expires | — |
| Older than `max_days` | Expires regardless of activity | — |

In-app **role** changes need none of this: roles are re-read from the database on
every request, so a grant or revoke applies immediately.

## Endpoints

| Endpoint | Auth | Purpose |
| --- | --- | --- |
| `POST /api/v1/auth/session` | Bearer or cookie | Mint the cookie (the token-for-cookie exchange). Idempotent: when the caller is *already* cookie-authenticated it just reports the new deadline instead of orphaning the current row. Returns `{expires_at}`. `501` when sessions are disabled. |
| `DELETE /api/v1/auth/session` | Cookie | Sign out here — **revokes server-side**, then clears the cookie. Clearing alone would leave a captured cookie working for the rest of its 60 days. |
| `DELETE /api/v1/auth/sessions` | Cookie or Bearer | Sign out everywhere. Returns `{revoked}`. |
| `DELETE /api/v1/users/{id}/sessions` | **admin** | End someone else's sessions. Returns `{revoked}`; `404` for an unknown id (rather than a misleading `{"revoked":0}`). Audited as `revoke_user_sessions` / `admin`. |
| `POST /api/v1/auth/backchannel-logout` | **the logout token itself** | The IdP's OIDC back-channel logout callback. Audited as `revoke_user_sessions` / `idp_logout`. |

`revoke_user_sessions` is the one new audit action. Sign-in and self-logout are
still not audited — the app does not audit its own logins.

## Back-channel logout

The only path by which an IdP-side sign-out reaches the app. Asgardeo POSTs a
signed **logout token** (form field `logout_token`) when a session ends there;
`handler/backchannel_logout.go` verifies it and revokes.

- The route is the **only one outside the authenticated group** — the caller is
  the IdP's server, with no cookie and no Bearer token. **The signed token is the
  credential**, so `middleware.VerifyLogoutToken` is the entire authorization
  check, and it is deliberately *not* `provider.Verifier`: an ID token and a
  logout token are different artifacts (a logout token carries `events`, must not
  carry `nonce`, and its `exp` is optional), so reusing the ID-token verifier
  would either reject valid tokens or accept an ID token as a logout instruction.
  It checks: signature against the IdP's JWKS, `iss`, `aud` contains our client,
  the `events` claim holds the back-channel-logout event, no `nonce`, `iat`
  present and within 5 minutes (replay bound) and not in the future, `exp` when
  present, and at least one of `sid`/`sub`. `logout_token_test.go` covers each
  rejection, including `alg=none` and a token signed by a foreign key.
- **`sid` first, `sub` as fallback.** A `sid` revokes just the browser whose IdP
  session ended; with no `sid` (or one recorded before migration `053`) the token
  means "this subject's session is over", so every session that user has is
  revoked.
- **200 even when nothing was revoked.** The IdP broadcasts to every registered
  app, so "nobody by that subject has signed in here" is normal, not a failure to
  retry. Failures are `400` with no detail echoed — an unverifiable token is
  indistinguishable from an attacker asking us to sign someone out — and the
  reason is logged server-side.
- **Front-channel logout was not used**: it only fires while the app is open in a
  browser tab, so it would miss exactly the offboarding case this exists for.

Registering the callback URL in Asgardeo is a deployment step — see
[deployment-guide.md](deployment-guide.md#frontend--choreo-web-app-dockerfile-build).
Until it is registered, nothing breaks; IdP logout simply isn't propagated, and
the sweep below plus the admin control and `max_days` are the backstop.

**Asgardeo caveat**: its console exposes logout URLs only for some application
templates, and **not for a single-page application** — reasonably, since a pure
SPA has no server to receive a callback. This app does have one (it is a BFF), so
the mechanism is right even though the console field is missing; it can be set
through the Management API on the app's OIDC inbound config. That gap is exactly
why the pull-based sweep below exists.

## IdP offboarding sweep

`internal/offboard` closes the gap that matters most — an account **deleted or
disabled at the identity server** keeping its app session — with no per-app IdP
configuration at all.

- **How**: every `interval_minutes` (default 10), list the users holding a live
  session (`UsersWithLiveSessions`), compare them against the SCIM directory
  snapshot the autocomplete pickers already cache, and revoke the sessions of
  anyone **absent** (deleted) or **`active: false`** (disabled). Matching is by
  lowercased email, like every other identity check in the app.
- **Costs no extra SCIM traffic**: it reads that same cache
  (`forceRefresh=false`). A stale snapshot is safe — it can hide a *new* deletion
  until the next run, but it can never invent one.
- **Sessions only, never deactivation.** Revoking is self-correcting: if the IdP
  account comes back, the person signs in again and nothing needs undoing. An
  automated in-app deactivation would need an automated reactivation to match, and
  would fight an admin who re-enabled someone by hand.
- **Requires SCIM.** With `scim.enabled: false` the directory falls back to the
  app's own users, so "missing from the directory" would be self-referential —
  the sweep refuses to run and says so at startup.
- **Guards against a bad snapshot**, because the failure mode is a mass logout:
  an empty snapshot, or one with no usable emails, is never acted on; and a run
  that would sign out more than half the signed-in users (once at least 5 are
  involved) refuses entirely and logs why. Even then the blast radius is bounded —
  revocation does not block signing back in.
- `dry_run: true` logs every account it *would* sign out and revokes nothing.
- Each revocation is audited as `revoke_user_sessions` / `idp_offboard` with a
  **NULL actor** — the trigger is the identity server, not a person — and the
  reason in the detail column.

What it deliberately does **not** cover: a user simply pressing sign-out at the
IdP. That is a session-level event with no trace in the directory, so only
back-channel logout can deliver it.

## Config (`config.yaml`)

The whole block is optional; omitting it gives 60-day Secure/Lax cookies.

```yaml
session:
  enabled: true            # false ⇒ Bearer-only, the pre-session behaviour (rollback switch)
  ttl_days: 60             # idle lifetime
  max_days: 90             # absolute ceiling from session start; 0 disables (warned at startup)
  cookie_secure: false     # dev over http://localhost only; true everywhere real
  cookie_samesite: "lax"   # "none" only for a cross-site API (implies secure)
  idp_offboarding:         # sign out accounts deleted/disabled at the IdP
    enabled: true          # no-op unless scim.enabled is also true
    interval_minutes: 10
    dry_run: false
```

`cookie_samesite: "none"` also marks the cookie **Partitioned** (CHIPS), without
which browsers that block third-party cookies drop it silently — which looks
exactly like "logged out again". Safari blocks it either way.

## Frontend

- `api/client.ts` — `apiCredentials()` decides whether the cookie is sent:
  **on** for a same-origin or same-host API (`apiBaseUrl` empty/relative, or
  `localhost:8081` from `localhost:5173`), **off** for a cross-site API unless
  `config.js` sets `apiAllowCredentials: true`. This matters because
  `credentials: 'include'` makes the browser *require*
  `Access-Control-Allow-Credentials` on every cross-origin response, so enabling
  it against a gateway that does not send that header breaks every request rather
  than just skipping the cookie. When cookies are off, the app authenticates with
  Bearer tokens exactly as before.
- A **401** dispatches a `session-expired` window event; `AuthContext` shows the
  login page with "Your session has expired" (ignoring the expected 401 from its
  own boot probe).
- `AuthContext` boot order: **cookie** (`getMe()`) → **IdP token still in this
  tab** (traded for a cookie by `startSession()`) → login page. A handshake ref
  de-duplicates concurrent mints (boot + `userLoaded`).
- Signed-in state is `authenticated`, **not** the OIDC user: in the steady state
  the tab holds no live token at all. `automaticSilentRenew` is **off** and the
  token-expiry event is no longer wired to a logout — that was the direct cause
  of the daily sign-outs.
- `logout()` calls `DELETE /auth/session` first, then clears local tokens.

## Deploying this

For the cookie to be usable at all, the SPA and the API must share an origin —
straight out of Choreo they do not (`*.choreoapps.dev` vs `*.choreoapis.dev`,
different sites). That is solved by building the web app from
**`webapp/Dockerfile`**, whose nginx **proxies `/api`** to the backend: the
browser sees one origin, so the cookie is a first-party `SameSite=Lax` cookie
that works in every browser including Safari, and no gateway change is needed.

The deployment settings that go with it — `apiBaseUrl: ''` in the mounted
`config.js`, `session.cookie_samesite: "lax"`, and the browser checks to run
after the cutover — are the checklist in
[deployment-guide.md → Frontend](deployment-guide.md#frontend--choreo-web-app-dockerfile-build).

The fallback, if the component is ever served by the static-web-app buildpack
instead (which ignores `nginx.conf`), is a cross-site cookie:
`cookie_samesite: "none"` + `cookie_secure: true`, `apiAllowCredentials: true` in
`config.js`, and a gateway that returns `Access-Control-Allow-Credentials`.
Safari drops it regardless, so that path degrades to Bearer tokens and short
sessions — which is also exactly what the app does when nothing is configured, so
shipping the backend change on its own is harmless.
