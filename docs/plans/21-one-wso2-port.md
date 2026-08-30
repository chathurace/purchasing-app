# Port the purchasing-app UI into One WSO2

## Context

**One WSO2** (`wso2-open-operations/one-wso2`, `webapp/`) is the unified internal portal: a
React 19 + Vite + Oxygen UI frontend monolith that surfaces several previously-separate apps as
**perspectives** over their own unchanged backends. Three apps are already in:

| Perspective / area | Source app | Backend |
| --- | --- | --- |
| Me → Leave | people-ops-suite `leave-app` | Ballerina, `/user-info` + privileges |
| Me → OPD / Credit card / Expense | digiops-finance (3 apps) | Ballerina, per-app `/user-info` |
| Marketing Ops | digiops-marketing | **Python/FastAPI, `/api/*`, `/api/me`** |
| People Ops, My profile | people-ops-suite `people-app` | Ballerina `/user-info` |

**Goal:** duplicate the purchasing-app UI as a **Procurement** perspective inside One WSO2,
leaving the standalone purchasing webapp running and unchanged. Both frontends talk to the same
purchasing backend, for the same users, at the same time, until One WSO2 is stable enough to
retire the standalone app.

**Constraint:** One WSO2 is owned by another team. We port *exactly the way the existing ports
did* — which, per the marketing-ops commits, does include touching a fixed, small set of shared
files (`config/`, `constants/`, `App.tsx`, `SideRail.tsx`, `api/http.ts`, `config.js.example`).
Anything beyond that list is a request to them, not a change we make.

Marketing Ops is the closest precedent and the template for this work: same shape of backend
(Go/FastAPI under `/api`, its own `/api/me` and its own role scheme, unrelated to people-app
privileges), and its commit message states the pattern outright — *"an external backend surfaced
as apps inside one perspective, screens re-implemented in Oxygen UI, and the backend's own roles
gating the rail. The backend is unchanged — this is a frontend migration."*

Purchasing differs from every app ported so far in one way that dominates this plan: **it is a
BFF with its own session cookie and its own Asgardeo client**, not a token-only SPA over a
Choreo-gateway backend.

### Where the work happens

The port is done on the fork **`chathurace/one-wso2`**, which is a fork of
`wso2-open-operations/one-wso2` and currently level with it.

```
origin    git@github.com:chathurace/one-wso2.git            # ours — push here
upstream  git@github.com:wso2-open-operations/one-wso2.git  # canonical — read only
```

Working branch `feat/procurement-perspective`, cut from `upstream/main` at `a9f5b7a`, per the
webapp README's rule (feature branch off `main`, never commit to `main`, rebase on upstream before
a PR). Rebase on `upstream/main` at the start of each phase — the shared files this port edits are
actively changing (see below).

---

## Part 1 — Sign-in and session management

This is the part with real design decisions in it. Everything else is mechanical.

### What each side does today

**purchasing-app** (`webapp/src/auth/`, `docs/sessions.md`):

1. `oidc-client-ts` `UserManager` with **its own Asgardeo SPA client**
   (`oHN7X5sG7W_fPVBKDwsbwvowThsa` in Choreo), its own `/login` and `/callback` routes,
   `prompt: "login"` on sign-in, `automaticSilentRenew: false`.
2. The **ID token** (not the access token) is the API bearer — the backend needs the `email`
   claim, which WSO2 access tokens often omit.
3. `POST /api/v1/auth/session` trades that ID token, **once**, for an HttpOnly
   `__Host-purchasing_session` cookie with a **60-day sliding / 90-day absolute** lifetime.
   From then on the cookie is the credential and the IdP is never contacted again. This exists
   specifically to stop the roughly-daily sign-outs Asgardeo's lifetimes caused.
4. The cookie only works because `webapp/Dockerfile`'s nginx **proxies `/api`** to the Choreo
   backend, making the API same-origin so the cookie is first-party `SameSite=Lax`.
5. Session teardown has four paths: in-app sign-out (`DELETE /auth/session`), admin
   "end sessions", OIDC **back-channel logout** (`sid`-scoped), and the **IdP offboarding sweep**
   (`internal/offboard`, revokes sessions for accounts deleted/disabled at the IdP).
6. A 401 dispatches a `session-expired` window event; `AuthContext` shows `/login` with an
   expiry notice.

**One WSO2** (`webapp/src/layouts/AuthGuard.tsx`, `api/`, `context/idle-timeout/`):

1. `@asgardeo/react` `AsgardeoProvider` with **One WSO2's own client**, scopes
   `openid email groups profile`. `AuthGuard` wraps every route: not signed in → stash the
   target in `sessionStorage.one_wso2_post_login_redirect`, call `signIn()`, restore after.
2. The **access_token** is the bearer on every call (`authedGet/Post/Put/Patch/Delete` in
   `api/http.ts`). `fetchWithReauth` retries a 401 **once** after `signInSilently()`, and only
   for replay-safe GETs.
3. **No cookies anywhere** — `fetch` is called without `credentials`, so it defaults to
   `same-origin` and no cross-site cookie is ever sent.
4. Session lifetime = **Asgardeo's**, refreshed silently. On top of that,
   `IdleTimeoutProvider` shows an "Are you still there?" dialog at 25 minutes idle and
   (when `ONE_WSO2_IDLE_AUTO_SIGN_OUT` is true) signs out at 30.
5. Sign-out is `useSecureSignOut()` — clear the React Query cache, fire `SIGNING_OUT_EVENT`,
   `signOut()`.

### Decision 1 — Delete the purchasing auth layer entirely; One WSO2 owns sign-in

Not negotiable: two OIDC clients cannot both own the browser session in one SPA, and One WSO2's
`AuthGuard` already runs before any of our routes mount.

**Dropped from the port:** `auth/userManager.ts`, `auth/AuthContext.tsx`, `auth/RequireAuth.tsx`,
`pages/LoginPage.tsx` (362 lines), `pages/CallbackPage.tsx`, `api/session.ts`, and the
`oidc-client-ts` dependency. `api/client.ts` is replaced (see Decision 3).

The user signs in once to One WSO2 and the Procurement perspective is simply there. There is no
purchasing-specific sign-in, callback route, or "session expired" screen.

### Decision 2 — One credential everywhere: the Asgardeo access token. The cookie session goes.

**Revised.** This plan originally kept the 60-day cookie session for the standalone app and
accepted a shorter session only inside the portal. That is no longer the shape, for two reasons
that emerged while doing Phase 0:

1. The purchasing backend is moving **behind the Choreo gateway, as the only path** — needed for
   the CSP (Decision 7) and to make the deployment look like every other integrated backend.
2. **The 60-day session is incompatible with a gateway that enforces OAuth.** In the steady state
   the standalone webapp sends *no* `Authorization` header at all — that is the entire design
   ("the IdP token is presented exactly once, to mint this cookie"). A gateway enforcing OAuth
   rejects every one of those requests. The session model does not degrade under enforcement, it
   stops working.

Confirmed as a project decision that **the 60-day session is not required**. So both front ends
converge on one credential:

| Piece | Where it lands |
| --- | --- |
| Credential | Asgardeo **access token** as `Authorization: Bearer`, from both front ends |
| Standalone webapp | switch `getAccessToken()` from `user.id_token ?? user.access_token` to the access token, and **enable silent renew** (see the catch below) |
| Portal | `useAccessToken()` + the shared `@api/http` helpers, unmodified — no `procurementFetch` |
| Backend verification | unchanged: it keeps verifying the forwarded token itself, with `additional_client_ids` (Decision 4) |
| `x-jwt-assertion` | **not used** (see Decision 3) |
| Cookie session | switched off by config, then deleted (Phase 6) |

Session lifetime becomes Asgardeo's, for everyone, renewed silently — the same footing as Leave,
OPD, Marketing Ops and every other integrated app.

#### The catch: silent renew becomes load-bearing

Removing the cookie **reintroduces the bug the cookie was built to fix**. `automaticSilentRenew`
is deliberately `false` today, and the reason is explicit in `auth/userManager.ts`: the ID token
is needed exactly once, so "letting it expire is no longer a logout." Once the cookie is gone, an
expired token *is* a logout — `getAccessToken()` returns null the moment it expires — so the
standalone app signs people out roughly daily again unless renewal is on.

**Confirm the standalone app's Asgardeo application issues refresh tokens to the SPA.** If it
does, this is a flag flip (`automaticSilentRenew: true`, which oidc-client-ts serves from the
refresh token). If it does not, it needs a registered silent-callback redirect URI — an Asgardeo
registration change, not a code change. The portal is unaffected: `signInSilently()` already does
this.

#### `session.enabled: false` is the migration lever, and it already exists

The rollback switch built with the session feature is exactly the migration path — no code change
required to turn the cookie off:

- `middleware.Authenticate` skips the cookie branch entirely (`if a.session.Enabled`).
- `POST /api/v1/auth/session` returns **501**, and `AuthContext.establishSession()` already
  catches that and falls back to `getMe()` on the Bearer.

So the auth change can be shipped, verified and reverted **independently of the networking
change**, which is why the sequence below does it first.

Also: **do not call `DELETE /api/v1/auth/session` on sign-out** from the portal. There is no
cookie to revoke, and `useSecureSignOut()` is the shared teardown.

### Decision 3 — Identity comes from the token, not from `x-jwt-assertion`

`middleware.Authenticate` needs an `email` to resolve the caller. Every other integrated backend
gets it from Choreo's `x-jwt-assertion` header rather than from the token — verified in
One WSO2's own source: people-app's Ballerina `JwtInterceptor` decodes that header and casts it to
`{email, groups}`, and of Marketing Ops, `apiConfig.ts` records that it *"authenticates purely
from the gateway's `x-jwt-assertion` header and never inspects the Asgardeo token itself."*

**Purchasing deliberately does not follow them.** Three reasons:

1. **It needs none of what the assertion is for.** Marketing Ops reads it because its
   authorization model *is* Asgardeo groups (`_capability_from_group` in its `shared/rbac.py`).
   Purchasing's roles live in its own Postgres, keyed on `user_id` and re-read every request, so
   from the assertion it would use exactly one field — `email` — and ignore `groups` entirely.
2. **It would add network-position trust.** Today every credential is either cryptographically
   verified or hashed against our own database. Trusting a header means the backend is only as
   safe as the guarantee that nothing can reach it off-gateway — a Choreo endpoint-visibility
   setting that is not expressed in this repo. Doing it safely means verifying the assertion's own
   signature against Choreo's per-environment JWKS: a second verifier, a second key source, and
   key rotation to track. That is not "read a header."
3. **`email` is available two cheaper ways**, below.

So the backend keeps verifying the forwarded Bearer token itself. Under an enforcing gateway that
is defence in depth rather than the only check, and it is already written and tested.

#### Where `email` comes from, in preference order

**1. Put `email` on the access token** (Asgardeo claim configuration, on both applications). Zero
code. Check this first — it is a console setting.

**2. Call the IdP's userinfo endpoint on first login only.** This composes with the
`user_identities` table from Phase 0: `Authenticate` needs an email **only on the cold path**, an
unknown subject. A known subject resolves out of `user_identities` with no email at all. So it is
*one* userinfo call per (person, application) pair for all time, after which the mapping is
recorded and never consulted again. ~20 lines using the `*oidc.Provider` already constructed in
`NewAuth` (it simply isn't retained on the struct today). No claim configuration, no header trust,
nothing spoofable.

Prefer 2 over the assertion if 1 is unavailable: it is smaller, and it cannot be forged.

**The guard rail makes either safe.** If the token carries no email-like claim and the subject is
unknown, `ProvisionUserOnLogin` returns `ErrNoEmailClaim` and the request is refused with a 403
naming the claims it looked at — rather than silently creating a second, roleless account beside
the person's real one. See Decision 5.

### Decision 4 — The backend must accept a second audience  ⚠️ blocker

`middleware.NewAuth` builds `provider.Verifier(&oidc.Config{ClientID: cfg.ClientID})` from a
single `oidc.client_id`. go-oidc requires `aud` to contain that value. **A token minted for One
WSO2's Asgardeo client will be rejected with `invalid token` on every request.**

Fix (backend, ours, small):

- Add `oidc.additional_client_ids: []string` to config. Build one verifier per configured client
  id and accept a token that any of them verifies — or equivalently use
  `oidc.Config{SkipClientIDCheck: true}` plus an explicit `aud ∈ allowlist` check. Prefer the
  multi-verifier form; `SkipClientIDCheck` is easy to get wrong.
- Log the effective list at startup, next to the existing `OIDC initialized` line, so a rejected
  token is diagnosable from Choreo logs alone (the existing `unverifiedTokenClaims` warning
  already prints the incoming `aud`).
- Config: `additional_client_ids: ["<one-wso2-asgardeo-client-id>"]` in `config.choreo.yaml`.

Do **not** try to solve this by adding audiences in Asgardeo. For ID tokens `aud` is the client
id; the extra-audience settings apply to JWT access tokens and API resources, and relying on them
couples us to IdP config another team owns.

### Decision 5 — Identity collision on `sub`  ⚠️ blocker, and the one that can lock users out

`ProvisionUserOnLogin(sub, email, name)` keys identity on the OIDC `sub`
(`users.sub NOT NULL UNIQUE`), and `users` also has `CREATE UNIQUE INDEX users_email_lower_key ON
users (lower(email))` (migration 015).

If Asgardeo issues a **different `sub`** for the same person under One WSO2's application — which
it does when an app's subject identifier is application-scoped rather than the org-wide user UUID
— then on first login from One WSO2:

1. no row matches `sub`,
2. no pending invite matches (the user already has a `sub`),
3. the `INSERT ... ON CONFLICT (sub)` fires, `sub` does not conflict, **`lower(email)` does** →
   unique violation → `500 internal error` → **the user cannot use the Procurement perspective at
   all**, and every parallel request on page load fails the same way.

**Step 1 — verify.** Sign in to One WSO2 in dev, decode the tokens in its Auth Debug Panel, and
compare `sub` with the `sub` on that user's `users` row from the standalone app. Asgardeo's
default is the org-wide user id, in which case they match and nothing more is needed.

**Step 2 — fix regardless, because dual-run demands it.** Even if the subs match today, a single
`users.sub` column cannot represent "this human signs in from two applications". Add a
`user_identities` table and let one user hold several subjects:

```sql
-- migration 0NN
CREATE TABLE user_identities (
    sub        TEXT        PRIMARY KEY,
    user_id    BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
INSERT INTO user_identities (sub, user_id) SELECT sub, id FROM users WHERE sub IS NOT NULL;
```

`ProvisionUserOnLogin` then resolves in this order: `user_identities.sub` → existing pending
invite by `lower(email)` → an **existing user with that `lower(email)`, whose new `sub` is
recorded as an additional identity** → brand-new user. `users.sub` stays as-is (it is read by
`internal/offboard` and the logout-token path) and keeps holding whichever subject was seen first.
Adopt-by-email is safe here because the email claim is issued by the same Asgardeo org that
already authenticates the standalone app.

Roles, budget-approver assignments, PR history and audit rows all hang off `users.id`, so a user
who signs in through either frontend sees the same account — which is the whole point of running
both.

### Decision 6 — Session teardown, once there is no session of our own

With the cookie gone (Decision 2), Asgardeo is the only session authority, for **both** front
ends. Most of the teardown machinery this app grew stops having anything to act on:

| Mechanism | After the cutover |
| --- | --- |
| `DELETE /auth/session` on sign-out | Not called by either front end. `useSecureSignOut()` (portal) / `logout()` (standalone) sign out of Asgardeo |
| Back-channel logout (`sid` → revoke) | **No-op** — it revokes cookie sessions and there are none. Signing out at the IdP now genuinely ends access, which is what the mechanism was approximating |
| IdP offboarding sweep (`internal/offboard`) | **No-op**, and *superseded*: a deleted or disabled Asgardeo account can no longer mint a token at all. That is strictly better than a ten-minute sweep, and it needs no SCIM |
| Admin **"End sessions"** (Users page) | **No effect.** Remove the control in Phase 6 rather than leave a button that does nothing |
| Admin **Deactivate** | **Unchanged and still immediate** — `is_active` is re-read every request, so `serveAuthenticated` refuses the caller on the Bearer path too. This is the control admins actually reach for |
| Idle timeout | Portal only: One WSO2's 25-minute warning / 30-minute optional sign-out. The standalone app has none and gains none |

The security trade, stated plainly: the session kill switch moves from our database to Asgardeo.
We lose the ability to end one person's app access without touching the IdP — except by
**deactivating** them, which still works instantly and is the control that was actually used. In
exchange, IdP-side offboarding becomes immediate rather than eventually-consistent.

`docs/sessions.md` has been updated to describe both front ends; it gets a second pass in Phase 6
when the cookie machinery is actually deleted.

### Decision 7 — The gateway, CORS and CSP  ⚠️ deployment prerequisites

#### The gateway is already in the path — what changes is enforcement

Worth being precise, because the plan originally mis-stated this. `webapp/public/nginx.conf`
already proxies `/api` to
`…prod.wdt.choreoapis.dev/llkq/purchasing-test-backend/v1.0` — **that is Choreo's API gateway.**
So "move the backend behind the gateway" is not a topology change. What changes is:

- the gateway starts **enforcing OAuth** rather than forwarding `Authorization` untouched, and
- the direct/off-gateway route is **closed**, making the gateway the only path.

Two consequences that are easy to miss:

- **`choreoapis.dev` does not satisfy One WSO2's CSP** (below). Being behind the gateway is not
  enough — the API has to be published on the **`apis[-stg].wso2.com`** host, which is where every
  other integrated backend lives. That is the actual deployment task.
- **"Only path" must be verified as a network guarantee**, not inferred from intent: Choreo
  endpoint visibility set to organization/project rather than public. It is not expressed in this
  repo, so check it in the console. (With Decision 3 the backend still verifies tokens itself, so
  this is not load-bearing for correctness — which is exactly why Decision 3 is worth keeping.)

#### What enforcement breaks, beyond the session

Decision 2 covers the cookie session. Three more, all needing a check before cutover:

1. **`POST /api/v1/auth/backchannel-logout`** is deliberately outside the authenticated group —
   the caller is Asgardeo's *server*, presenting a signed `logout_token` as a form field with no
   Bearer. An enforcing gateway rejects it. Moot once the cookie session goes (there are no cookie
   sessions to revoke), and the route is retired in Phase 6.
2. **Gateway payload and timeout ceilings.** `nginx.conf` raises `client_max_body_size` to **32m**
   and read/send timeouts to **300s** for a reason: quotation PDFs run to `anthropic.max_pdf_bytes`
   (20MB) and server-side extraction has a 180s cap. Choreo's gateway has its own limits, and
   these are a hard constraint independent of auth. **Check both before cutover** — this is the
   most likely thing to fail late and look unrelated.
3. **Streamed downloads.** nginx sets `proxy_buffering off` so protected file downloads stream
   through. Gateway buffering could break or badly delay large downloads.

#### CORS

Ours, config-only: add One WSO2's origins to `cors.allowed_origins` — `http://localhost:3000` for
dev, its Choreo web-app URL for staging/prod. `middleware.CORS` already echoes an allowed origin
and sends `Access-Control-Allow-Headers: Authorization, Content-Type`, which is all One WSO2
sends. **Done for dev**, verified with a preflight; the prod origin is outstanding.

The same list feeds `CheckCSRFOrigin`. That is harmless once the cookie path is gone, but the two
intentions are worth keeping distinct if the lists ever diverge.

#### CSP

One WSO2's production build injects `connect-src 'self' https://*.wso2.com https://*.asgardeo.io`.
Its own comment names the hosts this was written around: the Choreo gateway on
`apis[-stg].wso2.com` and Asgardeo. So:

1. **Preferred:** publish the purchasing API on `apis.wso2.com`, as above. No One WSO2 change, and
   it matches the pattern the CSP already anticipates.
2. Ask the One WSO2 team to widen `connect-src`. Slower, and it weakens a policy that is theirs.

Take (1). It is Choreo API management, not code, and it must land **before** the first production
deploy — dev is unaffected because the CSP is injected on production builds only.

**Second CSP casualty:** `components/StorageSettings.tsx` loads
`https://apis.google.com/js/api.js` and `https://accounts.google.com/gsi/client` and renders the
Drive Picker in a `docs.google.com` iframe. One WSO2's `script-src 'self'` and
`frame-src 'self' blob: data: https://*.asgardeo.io` block all of it, and widening `script-src`
for one admin screen is not a reasonable ask. **Excluded from the port** — it is admin-only and
configured once, and the Picker is not decoration: under the `drive.file` scope, selecting a folder
in Google's own Picker *is* the grant, so it cannot be replaced with our own folder browser
without escalating to a restricted scope. Storage stays configurable from the standalone app for
the dual-run; `SettingsPage` ports without that section. (The options, if it ever has to move
in-portal, are recorded in the file-storage discussion: a status-only panel that deep-links out, a
grant page hosted on the backend's own origin, or switching the deployment to
`service_account` mode.)

Document previews and downloads are fine: `downloadFile`'s blob + `<a download>` needs no CSP
allowance, and `frame-src blob: data:` already covers inline PDF previews.

### Decision 8 — Authorization: `/api/v1/me` gates the rail, exactly as Marketing Ops does

Purchasing's roles (`staff`, `procurement`, `procurement_admin`, `admin`, `legal`, `security`,
`compliance`) plus `is_approver` bear no relation to the people-app privilege numbers
(`987/993/991/999`) that `SideRail`'s generic `caps` check uses. Reading `requires` against
`caps` would show a people-app admin every procurement screen and hide them from an actual
procurement admin. `SideRail` already solves this twice:

```
features/procurement/api/usePurchasingMe.ts     — GET /api/v1/me, keyed on the Asgardeo sub
features/procurement/api/usePurchasingGate.ts   — canSee(itemId) from roles + is_approver,
                                                  reusing lib/nav/navModel.ts's permission
                                                  predicates so both frontends gate identically
```

and in `SideRail.tsx` — the one shared file this touches — three lines beside the existing two
gates:

```ts
const isProcurement = active.key === "procurement";
const procurementGate = usePurchasingGate(isProcurement);
// in resolveVisible: if (isProcurement) return procurementGate.canSee(s.id);
```

Fail closed on unmapped restricted ids, as marketing-ops does. A `ProcurementShell` (copy of
`MarketingOpsShell`) owns the four degraded states in one place: backend not configured,
resolving, request failed, not authorized — keeping "gateway broke" and "you lack the role"
distinct, which is the point that comment in `MarketingOpsShell` makes.

**Note on self-provisioning:** the backend creates a `users` row with the `staff` role on first
login. Exposing the perspective to every WSO2 employee therefore provisions a row for anyone who
clicks it. That is already true of the standalone app, and `getNavGroups` shows plain staff only
Home / My requests / Approvals, so this is acceptable — but expect the `users` table to grow and
the Users page to fill with `staff` rows.

---

## Part 2 — The UI port

### No new dependencies

Every external import in the purchasing webapp, across all 79 source files:

| Module | Status in One WSO2 |
| --- | --- |
| `@wso2/oxygen-ui` (46 named imports) | present (0.6.0 vs our 0.12.0 — see below) |
| `@wso2/oxygen-ui-icons-react` (35 icons) | present |
| `@tanstack/react-query` | present (5.90) |
| `react-router-dom` v6 | → `react-router` v7, specifier rewrite only |
| `oidc-client-ts` | **dropped** (Decision 1) |
| `@emotion/*` | transitive via Oxygen |

No charts library: `RateComparisonChart` and `PRAnalyticsPage` are hand-rolled SVG, so One WSO2's
`recharts` is not needed. `exceljs`, `html-to-text`, `parse5`, `react-window` in One WSO2 are
other features' and stay untouched.

**Oxygen 0.6.0 vs 0.12.0.** Of the 46 components used, 43 are plain `@mui/material`
re-exports (`export * from '@mui/material'` in Oxygen's `dist/index.d.ts`) and One WSO2 pins
`@mui/material` 7.3.4 directly, so `Table`, `TableSortLabel`, `Breadcrumbs`, `CardActionArea`,
`alpha`, `useMediaQuery` etc. resolve identically. The three Oxygen-specific ones —
`AppShell`, `Header`, `Sidebar` — are **shell chrome we delete anyway**, along with
`OxygenUIThemeProvider`, `WSO2Theme` and `Theme`. Nothing else to reconcile. Do not bump One
WSO2's Oxygen pin.

### What gets deleted rather than ported

| File | Lines | Why |
| --- | --- | --- |
| `App.tsx` | 84 | routes move into One WSO2's `App.tsx` |
| `main.tsx` | ~45 | One WSO2 owns the provider tree; the light-mode pin must go (see theming) |
| `App.css` | 33 | One WSO2 owns global styles and the background wash |
| `auth/*` (3 files) | ~360 | Decision 1 |
| `pages/LoginPage.tsx`, `pages/CallbackPage.tsx` | ~400 | Decision 1 |
| `api/session.ts` | 33 | Decision 2 |
| `api/client.ts` | 132 | replaced by the shared `@api/http` helpers (Decision 3) |
| `components/Layout.tsx` | 256 | One WSO2's `AppLayout` + `TopBar` + `SideRail` is the shell |
| `components/StorageSettings.tsx` | 269 | CSP (Decision 7) |
| `lib/nav/navModel.ts` | 129 | *converted*, not deleted — see below |

~1.7k lines dropped, **~19.3k lines ported** across ~68 files. The four biggest files —
`RecommendationSection.tsx` (2095), `PurchaseRequestDetailPage.tsx` (1118), `RequisitionForm.tsx`
(1018), `QuotationComparisonCard.tsx` (869) — are pure presentation over the API types and port
with only the mechanical rewrites below.

### Route namespace

Purchasing's routes are top-level (`/requests`, `/approvals`, `/vendors`, `/settings`, `/users`,
`/audit`) and would collide with One WSO2's namespace. Everything moves under `/procurement`:

```
/procurement                              HomePage (role-based dashboard)
/procurement/my-requests                  MyRequestsPage
/procurement/requests[/new|/:id]          list · create · detail
/procurement/approvals                    ApprovalsListPage
/procurement/quotations[/:id]
/procurement/contracts[/:id][/grns/new|/invoices/new]
/procurement/grns[/:id]
/procurement/invoices[/:id]
/procurement/vendors[/:id]
/procurement/business-units[/:id]
/procurement/analytics/purchase-requests[/:id]
/procurement/users · /audit · /settings
```

31 absolute path literals in JSX/`navigate()` calls, plus `navModel.ts`'s `to` fields, need the
prefix. Introduce `const PROCUREMENT_BASE = "/procurement"` and a `p("/requests/…")` helper in
`features/procurement/constants/routes.ts` rather than hand-editing string literals — the 31
call sites become mechanical and future moves are one edit.

### Registry and rail

`lib/nav/navModel.ts` becomes `constants/procurementApps.ts` in One WSO2's `MenuApp[]` shape
(the same transform Marketing Ops applied), grouped as the nav model already groups:

- **Requests** — Home, My requests, New request, Approvals *(everyone)*
- **Procurement** — Purchase requests, Quotations, Contracts, GRNs, Invoices *(`procurement`)*
- **Analytics** — Purchase requests *(`canViewAnalytics`)*
- **Admin** — Vendors, Business units, Users, Audit log, Settings *(per-flag)*

`requires` on each item stays as a coarse hint (One WSO2's registry documents it as exactly
that); `usePurchasingGate` is what actually decides, per Decision 8. Keep
`getNavGroups`'s predicates as the shared source of truth for both frontends so a role change is
made in one place.

Then, in `constants/perspectives.ts`, a new functional perspective. Note `externallyGated` —
added upstream in `a9f5b7a` and **required here**, for exactly the reason Marketing Ops sets it:
`access: true` means "it is built", not "whoever is looking may use it", and the landing page must
not drop a user who holds no procurement role onto an authorization notice at login. Surfaces the
user drives (rail, launcher, favourites) still show it, because the gate's own message is the
right answer there.

```ts
{ key: "procurement", label: "Procurement", icon: ShoppingCartIcon,
  group: "functional", access: true, externallyGated: true,
  path: "/procurement", sections: appsToSections(PROCUREMENT_APPS) }
```

Registration also now requires a **`PERSPECTIVE_HUES` entry** in `config/perspectiveHues.ts` —
a base hue plus explicit light *and* dark `{bg, fg}` tints, used to tint the rail, the launcher
tiles and the active-group icon. Every registered perspective has one; omitting it degrades
silently (`perspectiveHue()` returns `undefined`) rather than failing, which is easy to miss.
Pick a hue not already taken by Me (orange), People (blue), Finance (green), Workspace (violet),
Marketing (pink) or Requests (gold).

`reachablePerspectives()` picks the perspective up automatically once `access` and `path` are set,
which means it also becomes a **landing-page option** and a **launcher favourite** with no extra
work — and, via `externallyGated`, is correctly excluded from the landing options a user can be
sent to without clicking.

**Open decision — dual-surfacing under Me.** Leave, OPD, credit-card and expense claims live
under **Me**, on the stated rationale that they are things an employee does for themself. "My
requests", "New request" and "Approvals" fit that description exactly. But the rail navigates by
absolute `path`, so a Me rail item pointing at `/procurement/my-requests` would flip the active
perspective to Procurement mid-click — a wrinkle no existing port has. **Recommendation: ship
Phase 1 as a single Procurement perspective**, then take dual-surfacing to the One WSO2 team as
its own change once the port is stable. Flagging it rather than deciding it, because it is an IA
call that affects their shell.

### Mechanical rewrites, per file

1. `from "react-router-dom"` → `from "react-router"`. `Link`, `Navigate`, `Outlet`,
   `useLocation`, `useNavigate`, `useParams`, `useSearchParams` are API-compatible in v7.
2. `import { … } from "../api/client"` → One WSO2's shared `@api/http` helpers
   (`authedGet`/`authedPost`/…), with `fetchWithReauth` for binary and multipart traffic as
   `features/finance/util/financeReceipts.ts` already does;
   every `api/*.ts` module gains the access token as a parameter, following how
   `useMarketingOpsSettings` threads `accessToken` through.
3. `useAuth()` → drop, or `useSecureSignOut()` where it was `logout()`. The one `UserMenu`
   consumer dies with `Layout.tsx`.
4. `useMe()` → `usePurchasingMe()`; the six `useCanX` hooks re-express as `usePurchasingGate`
   selectors over the same `getNavGroups` predicates.
5. Absolute paths → `p()`.
6. Page chrome: each page currently renders bare into `Layout`'s `<Outlet/>`. One WSO2 pages lead
   with `PerspectiveHeader` / a `ProcurementShell` wrapper (eyebrow + `h1` + subtitle), and
   `AppLayout` already supplies `p: 3` padding — so strip per-page top padding and let the shell
   own the heading. This is the one place the port is a *rewrite* rather than a move.

### Theming — the one real visual risk

The standalone app **pins light mode before React mounts** (`localStorage["mui-mode"] = "light"`,
`data-color-scheme="light"`) with a comment that the redesign is light-only. One WSO2 is
**theme-aware, and got more so in `a9f5b7a`**: a user-facing theme picker with five options —
Acrylic Orange (default), Acrylic Purple, Choreo, Classic, High Contrast — each in light *and*
dark, plus an accessibility overlay (`config/a11yThemeOverrides.ts`) that shifts text/outlined
`color="primary"` controls to `primary.dark` in light mode for WCAG AA.

So the surface to verify is **ten combinations**, not one, and High Contrast is the one most
likely to expose a hardcoded value. The pin cannot come along — it would break every theme but
one for the whole portal. So:

- Audit hardcoded colors. Good news: only **4 files, ~15 hex literals** (`#fff` ×9, plus orange
  `#eb6834`/`#d95926`, blue `#3987e5`/`#2a78d6`, green `#0e9f6e`) and **zero** `rgba()` uses.
  Replace with theme tokens (`common.white`, `primary.main`, `info.main`, `success.main`) or
  `alpha(theme.palette.…)`.
- Re-check every status `Chip`, `Alert` and the SVG charts in both schemes. `RateComparisonChart`
  and `PRAnalyticsPage` draw their own strokes and fills and are the likeliest to render
  invisibly on a dark canvas.
- `RequisitionForm`'s scoped `.proq-req` CSS island (recolored to orange in plan 19) needs the
  same pass; convert it to `sx` if it fights the theme rather than shipping a second CSS file.

### Config and docs

`webapp/public/config.js.example` + the `Window["config"]` declaration in `config/authConfig.ts`
gain one key, following the established comment style:

```js
// purchasing-app backend base URL. A Go service under /api/v1/*, with its own
// role scheme (/api/v1/me) — unrelated to the people-app privileges the rest of
// the app gates on. Optional — when absent, the Procurement screens show a
// "not connected" state.
// ONE_WSO2_PURCHASING_BACKEND_URL: "<your-purchasing-backend-url>",
```

and `config/apiConfig.ts` gains `purchasingBackendUrl`,
`isPurchasingBackendConfigured()` and a `purchasingServiceUrls` map covering the ~20 endpoint
families in `api/*.ts`.

---

## Files touched in One WSO2

Exactly the set the marketing-ops commits touched — nothing outside it:

| Shared (small, prescribed edits) | Change |
| --- | --- |
| `public/config.js.example` | one documented key |
| `src/config/authConfig.ts` | one `Window["config"]` field |
| `src/config/apiConfig.ts` | `purchasingBackendUrl` + service URLs |
| `src/constants/procurementApps.ts` | **new** — the `MenuApp[]` registry |
| `src/constants/perspectives.ts` | register the perspective (incl. `externallyGated`) |
| `src/config/perspectiveHues.ts` | one hue entry, light + dark tints |
| `src/App.tsx` | ~20 routes |
| `src/components/side-rail/SideRail.tsx` | 3 lines — the gate dispatch |
| `src/api/http.ts` | only if a verb is genuinely missing (probably not) |

| Ours alone | Change |
| --- | --- |
| `src/features/procurement/**` | ~68 ported files, ~19.3k lines |

Backend (our repo): multi-audience OIDC verification, `user_identities` migration +
`ProvisionUserOnLogin`, CORS origins, `docs/sessions.md` addendum, `config.choreo.yaml`.

---

## Phasing

Each phase is independently mergeable and leaves One WSO2 shippable. Feature branch off `main`,
rebase before PR — never commit to `main`.

**Phase 0 — Unblock (backend + Choreo; no One WSO2 code).** Do this first; the rest is wasted
effort until it lands.

| # | Item | State |
| --- | --- | --- |
| 1 | Multi-audience OIDC verification + config (Decision 4) | **done** |
| 2 | `user_identities` migration + `ProvisionUserOnLogin` rework (Decision 5) | **done** |
| 3 | Guard rail: refuse to create a user with no email claim (Decisions 3, 5) | **done** |
| 4 | CORS origins (Decision 7) | **done** — dev, plus the deployed portal origin `https://one-stg.wso2.com` |
| 5 | Does `email` ride on the access token? (Decision 3) | **done — yes.** See the token analysis below |
| 6 | Does the standalone app's Asgardeo application issue **refresh tokens** to the SPA? (Decision 2) | **needs the Asgardeo console** |
| 7 | Does the gateway **forward `Authorization`**, and accept an Asgardeo access token? (Decisions 2, 3) | **accepts: yes, proven.** Forwarding: still unknown — `apis-stg` cannot reach upstream (503). See the gateway probe below |
| 8 | Gateway **payload / timeout ceilings** ≥ 32MB and 300s? (Decision 7) | **needs Choreo** |
| 9 | Publish the API on `apis[-stg].wso2.com` (Decision 7) | **done** — `https://apis-stg.wso2.com/llkq/purchasing-test-backend/v1.0` |

#### Re-probe after the redeploy (2026-08-30)  — two SEPARATE blockers

| Probe | Result | Reads as |
|---|---|---|
| `apis-stg` + portal Bearer | **503** `102503`, ECONNREFUSED | Unchanged. Gateway auth still passes; upstream still unreachable |
| Direct `*.choreoapis.dev` + portal Bearer | **401** `invalid token` | The RUNNING backend still does not accept the portal audience |

These are independent, and neither is a code problem.

**1. The config change never reached Choreo.** `config.choreo.yaml` is *gitignored*,
and the backend is built by Choreo's Go buildpack **from the repo** (`project.toml`,
`GOOGLE_BUILDABLE=./cmd/server`). So that file is not in the build at all. Per
`docs/deployment-guide.md`, runtime values are supplied "through the runtime
`config.yaml` / config-map, not committed to the repo". Choreo holds its OWN copy.
Editing the local file — which is what was done here — changes nothing that runs.
The `additional_client_ids` block has to be pasted into the Choreo configuration and
the component redeployed. Until then the portal audience is rejected, exactly as
observed.

> Worth designing around later: a gitignored config that must be hand-mirrored into
> a console has no mechanism to tell you it has drifted. The `invalid token` here was
> indistinguishable from a code bug until the direct URL isolated it.

**2. `apis-stg` cannot reach the service.** Separate from the above and unaffected by
it: the gateway authenticates the caller, then fails to connect upstream on every
path. The service is demonstrably alive on the direct URL at the same moment. This
is the API's endpoint configuration or the component's deployment in the environment
the vanity host maps to.

**Order matters.** Fixing (2) alone still yields `invalid token`; fixing (1) alone
still yields 503 through the gateway. Both must land before the portal can call the
backend over `apis-stg`, and only then can the `Authorization`-forwarding question
be answered.

#### Gateway probe with a real portal token (2026-08-30)  ⚠️ blocker found

Run against `apis-stg` with a valid One WSO2 access token. Item 7 is now half
answered, and a **new deployment blocker** surfaced.

| Probe | Result |
|---|---|
| `GET /api/v1/me`, no auth | **401**, `900901` "Invalid Credentials" — the gateway's own |
| `GET /api/v1/me`, portal Bearer | **503**, `102503` "Upstream connection failed" |
| Same, every other path (`/home`, `/healthz`, `/`) | **503**, identical |
| Old `*.choreoapis.dev` URL, no auth | **401** `missing authorization header` — OUR backend, alive |
| Old URL, portal Bearer | **401** `invalid token` — expected, see below |

**The gateway ACCEPTS the Asgardeo token.** The unauthenticated control is refused
by Choreo at the edge; the authenticated one gets *past* auth and fails only when
the gateway dials upstream. So the `choreo:deployment:sandbox` audience matches the
environment behind `apis-stg` — that worry is retired. The gateway also emitted
`access-control-allow-origin: https://one-stg.wso2.com`, so the portal origin is
fine at the edge.

**⚠️ But the upstream is unreachable from `apis-stg`.** `delayed connect error: 111`
is ECONNREFUSED — not DNS, not a timeout: the gateway dials an address where nothing
is listening. It is not path-specific (`/`, `/healthz`, `/api/v1/*` all 503), so it
is the API's endpoint configuration, not routing within our service.

**Our service is healthy.** The same backend answers on the old
`*.choreoapis.dev` URL with its own plain-text errors. So the deployed binary is
running; only the `apis-stg` publication cannot reach it. Two things to check in
Choreo, in order: whether the API's endpoint URL is right, and whether the component
is actually deployed in the environment the vanity host maps to.

**The deployed backend still rejects the portal token** (`invalid token` on the old
URL). Expected — `additional_client_ids` has been added to `config.choreo.yaml` but
the backend has not been redeployed. It confirms the change is both necessary and
not yet live.

**Still unknown: whether the gateway forwards `Authorization`.** Unanswerable until
upstream is reachable — the request never arrives at our code. This is the one
question that could still force reading `x-jwt-assertion` as a carrier (verifying it
ourselves, per Decision 3).

#### What a real One WSO2 access token showed (2026-08-30)

A live token from `https://one-stg.wso2.com`, decoded locally. This settles several
things that were guesswork, and raises one new one.

**It passes our verifier.** RS256 JWT, `typ: at+jwt` (go-oidc does not inspect
`typ`, so an *access* token goes through the ID-token verifier unremarked), `iss`
exactly our configured issuer, and `aud` carrying the portal's client id.

**`aud` is an ARRAY, not a string:**

```json
"aud": ["YW2Q2KbBWLYB5_AkKxSN3U8WSHUa", "choreo:deployment:sandbox"]
```

Every audience test written before this used a single string. Two cases were added
for the real shape — array-containing-a-configured-client accepted, array-with-no-
configured-client rejected. The second is the one that matters: every Choreo app in
the organisation carries a `choreo:deployment:*` marker, so a membership test that
degenerated into "the array is non-empty" would authenticate all of them.

**`email` is present on the access token** (item 5, closed). So Decision 3 needs no
userinfo round trip, and the Decision 5 guard rail will not trip for portal users.

**⚠️ The token is bound to a Choreo ENVIRONMENT** via `choreo:deployment:sandbox`.
If the purchasing API is published to a *production* Choreo environment, the gateway
refuses this token whatever we configure — the rejection happens before our code
runs. `one-stg` and `apis-stg` are both staging, so they plausibly match, but this
is unverified and is now the critical path for item 7.

**Token lifetime is 60 minutes.** With `session.enabled: false` already live, item 6
(silent renew) is not a future concern: standalone users are being signed out hourly
today.

**No `name` claim — fixed.** The access token carries `given_name` and
`family_name` but no `name`, which the middleware was reading alone. Portal users
were provisioned with a blank name (verified locally: user 3824, `name` empty).
`auth.go` now composes `given_name + family_name` when `name` is absent, preferring
`name` when present and still falling back to the username claims last. The
standalone app is unaffected — its ID token has `name`, which still wins.

**The Asgardeo application is "One WSO2 App Frontend_Sandbox"**, which is where the
`choreo:deployment:sandbox` audience comes from. `http://localhost:3000` is a
registered callback on it (probed against the authorize endpoint; an unregistered
URI returns `invalid_callback`), so the portal can be run locally against a local
backend without any Asgardeo change.

**Local end-to-end is proven** (2026-08-30), gateway excluded: backend on :8099 with
`oidc.issuer` pointed at the real Asgardeo tenant and both client ids accepted,
`GET /api/v1/me` returns 200 with roles, `/api/v1/home` 200, CORS echoes an allowed
origin and withholds the header from an unlisted one. What this does NOT cover is
the gateway: the sandbox-audience question and `Authorization` forwarding are still
only answerable against `apis-stg`.

**Node 20.19+ is required** to run the portal at all (Vite 7 uses `crypto.hash`).
Node 18 fails at dev-server start. This also retires an earlier note in this plan:
the "repo-wide jsdom vitest failure" was a Node 18 artifact — on Node 22 the full
suite passes (14 files, 154 tests).

**`sub` is a tenant user UUID**, not visibly application-scoped. Whether it matches
the subject the standalone app sees is still unknown and does not need to be known —
identical, and `user_identities` resolves it; different, and adopt-by-email links it.
Confirm on staging after the first portal sign-in: expect ONE `users` row for the
email, with two `user_identities` rows if the subjects differ.

#### What probing the published URL established (2026-08-28)

- **The gateway enforces OAuth.** Both an unauthenticated request and one with a
  junk Bearer are refused by *Choreo*, not by us:
  `www-authenticate: Bearer realm="Choreo Connect"`, body `{"code":"900901", …}`.
  Our own refusal is plain text (`missing authorization header`), so the
  responder is unambiguous. Half of item 7 answered: enforcement is on. What is
  still unknown is whether the gateway accepts an **Asgardeo** token (rather than
  a Choreo-issued one) and whether it forwards `Authorization` to the backend —
  both need a real token from the portal.
- **The gateway answers CORS preflight itself.** `OPTIONS` from
  `http://localhost:3000` → 200 with its own `Access-Control-*` headers, echoing
  the Origin it was given. Our `middleware.CORS` is therefore bypassed for
  gateway traffic and governs only the direct route.
- **The old `*.choreoapis.dev` URL is still open and NOT gateway-enforced** — it
  returns our backend's own `missing authorization header`. See the risk table.
- **`session.enabled: false` is set on staging**, so the backend is already
  Bearer-only there. Phase 4.5 step 4, done ahead of the sequence. The standalone
  webapp still works because its nginx proxies to the *old*, unenforced URL and
  it still holds a live ID token; `POST /auth/session` now 501s and
  `AuthContext.establishSession()` falls back to `getMe()`, which is the designed
  degradation. **Its sessions are now Asgardeo-length**, so item 6 (silent renew)
  is no longer hypothetical — that is the live risk on staging today.

*Exit criterion:* an access token minted by One WSO2's client returns a valid `GET /api/v1/me`
through the gateway, for a user who already exists in the purchasing database **and** for one who
does not.

Items 1–3 were built to be correct however 5–8 resolve. Item 2 especially: one `users.sub` column
cannot represent "signs in from two applications" regardless of whether the subjects happen to
match today. Item 5 decides only *where* `email` comes from (claim config vs a first-login
userinfo call — Decision 3), and item 3 makes a wrong answer loud instead of silent either way.

Items 5–8 are all console questions, and answering 7 early is worth prioritising: if the gateway
forwards `Authorization` and accepts an access token, no `x-jwt-assertion` work is needed at all
and the backend is finished.

As built (backend):

- `migrations/054_user_identities.sql` — subject → user, backfilled from `users.sub`. `users.sub`
  left in place as the primary subject, because `pending` is derived from it.
- `ProvisionUserOnLogin` resolves subject → invite → **adopt-by-email** → new user, recording an
  identity on every path. The adopt step is what prevents the lockout; it is audited as
  `link_identity` with a NULL actor. An unknown subject takes a `pg_advisory_xact_lock` on the
  email first, so two front ends firing parallel requests on a first login cannot race into a
  `users_email_lower_key` violation.
- `RevokeUserSessionsBySub` and `UpsertUser` both go through the new table, so no write path
  leaves a subject that cannot be resolved.
- `oidc.additional_client_ids` — one full verifier per accepted audience, own client first;
  `accepted_client_ids` logged at startup and beside every rejection. Back-channel logout stays
  bound to `oidc.client_id`.
- Tests: 7 audience cases (`auth_audience_test.go`, incl. foreign audience, foreign signing key,
  expired-on-the-additional-audience, and logout tokens *not* inheriting the widening) and 6
  identity cases (`user_identities_integration_test.go`, incl. the lockout scenario and the
  concurrent-unknown-subject race). Full backend suite green.
- `docs/sessions.md` gained a "second front end" section covering all of it.

**Phase 1 — Foundation.** Perspective registration, hue, routes, the `@api/http` call layer,
`usePurchasingMe`/`usePurchasingGate`, `ProcurementShell`, `SideRail` dispatch, config plumbing,
and **HomePage + MyRequestsPage + ApprovalsListPage** — enough for a plain staff user to do the
thing most staff do. Proves auth, gating and theming end to end on three screens instead of
sixty.

**Phase 2 — Procurement core.** Purchase requests (list, detail, `RequisitionForm`), quotations
(incl. `QuotationComparisonCard`, extraction review), `RecommendationSection`, `ProcessFlow`
timeline, chain stepper. The bulk of the lines and the bulk of the review.

**Phase 3 — Downstream documents.** Contracts, GRNs, invoices, vendors, business units.

**Phase 4 — Analytics and admin.** BPM analytics pages, Users, Audit log, Settings
**minus file storage**.

**Phase 4.5 — Auth cutover (backend + standalone webapp; no One WSO2 code).** The move to one
credential (Decision 2). Deliberately ordered so the auth change and the networking change can each
be reverted alone:

1. Confirm items 5–8 above.
2. `email` sorted — claim config, or the first-login userinfo call (Decision 3).
3. Standalone webapp: send the **access token**, and **enable silent renew**. Ship and watch —
   this is the step that decides whether people stay signed in.
4. Flip **`session.enabled: false`**. Config only, instantly reversible. Verify both front ends in
   staging: sign-in, a write, a 32MB upload, a streamed download.
5. Enable **gateway enforcement** and close the off-gateway route.
6. Verify again, in production, before touching any code.

Steps 3–4 are the risky ones and they are pure config plus a one-line frontend change. Step 5 is
the irreversible-feeling one, and by then nothing depends on the cookie.

**Phase 5 — Hardening.** Full theme pass across all five themes in both schemes (ten
combinations), High Contrast first; keyboard/screen-reader pass on the ported forms
(One WSO2 has an explicit a11y posture and the ported pages have never been checked against it);
Vitest coverage for `usePurchasingGate` and the route-prefix helper, matching how One WSO2 tests
`idleConfig` and the pinned store.

---

**Phase 6 — Remove the session machinery.** Only after the portal is stable: until then the
cookie layer is the rollback path, and `session.enabled: true` restores it with no deploy.

Backend: `middleware/session.go`, `handler/auth_session.go`, `handler/backchannel_logout.go` +
`middleware/logout_token.go` (and its tests), `internal/offboard`, `pruneSessions` in
`cmd/server`, the `session:` config block, and the three `/auth/session*` routes plus
`DELETE /users/{id}/sessions`. Migrations `052`/`053` stay — dropping `user_sessions` is a separate,
later call once nobody needs the forensic rows.

Frontend (standalone): `api/session.ts`, `apiCredentials()` / `sessionCookiesUsable()` in
`api/client.ts`, and most of `AuthContext`'s three-way boot — with no cookie there is one path,
"does this tab hold a live token".

UI: remove the Users page's **End sessions** control rather than leave a button that does nothing.
`revoke_user_sessions` stays in `ValidAuditActions` so historical audit rows still render.

Keep: `user_identities` and `additional_client_ids` (both still load-bearing), the deactivation
gate, and `link_identity`.

Second pass on `docs/sessions.md` — at that point it stops being "how our 60-day session works"
and becomes "why we don't have one".

## Risks

| Risk | Severity | Handling |
| --- | --- | --- |
| Asgardeo issues a different `sub` per application | **Critical** — 500 on every request, total lockout | Verify in Phase 0; `user_identities` fixes it either way and is required for dual-run regardless |
| Backend rejects One WSO2's audience | **Critical** — nothing works | Phase 0, Decision 4 |
| Production CSP blocks `*.choreoapis.dev` | **High** — works in dev, dead in prod | Move behind `apis.wso2.com` in Phase 0; do not discover this at deploy |
| 60-day session dropped for **everyone** | **Medium**, accepted | Decision 2. The live risk is not the length but **silent renew**: if the standalone app's Asgardeo application does not issue refresh tokens, step 3 of the cutover reintroduces daily sign-outs. Confirm before, not after |
| **The pre-vanity `*.choreoapis.dev` URL still reaches the backend, un-enforced** | **Medium** | Not an authentication bypass — the backend verifies the Asgardeo token itself on that path too, which is exactly why Decision 3 kept that check rather than trusting gateway headers. What it does mean: "the gateway is the only path" is **false today**, so gateway rate-limiting and observability can be walked around, and any future move to trust `x-jwt-assertion` would become a real privilege-escalation hole while it stays open. Close it (Choreo endpoint visibility) as part of Phase 4.5 step 5 — and re-verify with a curl, not by reading the console |
| Gateway payload/timeout ceilings below 32MB / 300s | **Medium** | Checked in Phase 0 item 8. Fails late and looks unrelated to auth — quotation uploads and PDF extraction are what break |
| Dark mode regressions in ported screens | Medium | 4 files of hex literals is small; the SVG charts are the real work — Phase 5 |
| Oxygen 0.6.0 lacks something 0.12.0 has | Low | 43/46 imports are MUI re-exports; the other 3 are deleted chrome |
| **Upstream drift in the shared files** — 17 commits landed while this plan was being written, touching `perspectives.ts`, `SideRail.tsx`, `App.tsx`, `config.js.example`, `authConfig.ts` and the whole theme layer | **Medium, ongoing** | Rebase on `upstream/main` at the start of every phase, not just before the PR. Keep each phase small enough to rebase cheaply — this is the main argument for the phasing below |
| Two frontends drift as purchasing keeps shipping | **Medium, ongoing** | Keep `getNavGroups`, `types/api.ts` and the permission predicates as the shared contract, and treat any change to them as needing both frontends updated in the same PR |
| Self-provisioning inflates `users` | Low | Already true standalone; plain staff see three nav items |

## Out of scope

- Any change to One WSO2 beyond the eight prescribed files.
- Retiring the standalone purchasing webapp — a later decision, once One WSO2 is stable.
- Porting the file-storage settings panel (CSP).
- Sabbatical-style deep-links back into the standalone app: not needed, since every screen except
  file storage is ported.
- Novera / Ask One integration for procurement data.
