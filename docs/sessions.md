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

## Endpoints

All three sit inside the authenticated route group.

| Endpoint | Purpose |
| --- | --- |
| `POST /api/v1/auth/session` | Mint the cookie (the token-for-cookie exchange). Idempotent: when the caller is *already* cookie-authenticated it just reports the new deadline instead of orphaning the current row. Returns `{expires_at}`. `501` when sessions are disabled. |
| `DELETE /api/v1/auth/session` | Sign out here — **revokes server-side**, then clears the cookie. Clearing alone would leave a captured cookie working for the rest of its 60 days. |
| `DELETE /api/v1/auth/sessions` | Sign out everywhere. Returns `{revoked}`. This is the containment lever that makes a long-lived cookie safe to hand out. |

No new process/audit event action — the app does not audit logins today.

## Config (`config.yaml`)

The whole block is optional; omitting it gives 60-day Secure/Lax cookies.

```yaml
session:
  enabled: true            # false ⇒ Bearer-only, the pre-session behaviour (rollback switch)
  ttl_days: 60             # idle lifetime
  cookie_secure: false     # dev over http://localhost only; true everywhere real
  cookie_samesite: "lax"   # "none" only for a cross-site API (implies secure)
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
