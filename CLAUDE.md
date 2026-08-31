# Global instructions

Provide short answers and explanation. Do not provide internal thinking process unless explicitly asked.

# Purchasing App — contributor onboarding

Internal app to manage the company's purchasing function. Greenfield sibling of `../finance-apps`
(same stack), but simpler: a CRUD app for purchase requests with role-based views and local file
storage. No workflow engine.

## Stack

- **Backend** (`backend/`): Go 1.25 · `go-chi/chi/v5` · `jackc/pgx/v5` (pgxpool) ·
  `coreos/go-oidc/v3` (SSO) · `anthropics/anthropic-sdk-go` (quotation PDF extraction) ·
  `rs/zerolog`. Config from **`config.yaml`** (not env vars).
- **Frontend** (`webapp/`): React 18 + TypeScript · Vite · React Router v6 · TanStack Query v5 ·
  `oidc-client-ts` · Tailwind CSS.
- **DB**: local PostgreSQL, database `purchasing`.
- **Files**: stored on local disk under `storage.files_root` (default `purchasing-app/files`),
  original filenames preserved.

## Roles (maintained in-app, not from the SSO token)

`staff`, `procurement`, `procurement_admin`, `admin`, plus `legal` / `security` / `compliance` (the
team approval cards on a PR's procurement recommendation; they also retain read access to contracts). New SSO
users are auto-provisioned as `staff` on first login. The `bootstrap_admin.email` in `config.yaml` is
granted `admin` on every login. Admins manage users from the **Users** page — see
`docs/user-management.md`.

## Conventions

- **Plan archival**: before approving a new Claude Code plan, copy `~/.claude/plans/<id>.md` into
  `docs/plans/NN-short-name.md` (Claude Code overwrites its active plan each session, so this is the
  only history). After a large phase, add a companion `docs/<topic>.md` as-built reference.
- **Migrations**: numbered `NNN_description.sql` under `backend/migrations/`. Apply with
  `psql -h localhost -d purchasing -f <file>`.
- **Config**: `backend/config.yaml` is gitignored; `config.example.yaml` is the committed template.
- **Local Postgres only** for dev. DB: `postgres://chathura@localhost:5432/purchasing?sslmode=disable`.
- **Remove/delete guard**: no destructive control fires straight from a click. Every remove/delete —
  and every *deactivate*, which is how vendors/business units/users/config options are retired since
  they can't be deleted — goes through the shared modal: `useConfirmAction()` in
  `components/ConfirmDialog.tsx` returns `[confirmNode, ask]`; render `confirmNode` in the component
  and call `ask({title, message, confirmLabel, onConfirm})` from the button (`danger` defaults to
  true, so the confirm button is red). Name the thing being destroyed in the message and say what
  else goes with it (child rows, files, a status rewind). Guard only the destructive direction —
  reactivate/add needs no modal — and *don't* guard form-local row removals (a draft approver chip, a
  line item) that Cancel already undoes. `window.confirm` is not used anywhere; the reversible
  reject/regenerate flows use `ConfirmDialog` directly since they track `busy`.

## Quick start

```bash
# DB
createdb purchasing
for f in backend/migrations/*.sql; do psql -h localhost -d purchasing -f "$f"; done

# backend (reads ./config.yaml, or -config <path>)
cd backend && go run ./cmd/server

# frontend
cd webapp && npm install && npm run dev   # http://localhost:5173
```

## Data clean up

Whenever test data is added by claude, clean up all those test data after testing.

## Status

- **Phase 1 (staff view)** — see `docs/plans/01-staff-view.md`. Pages: login, list requests, new
  request, view/edit request. The new-request form is the **ProQ requisition wizard**
  (`components/RequisitionForm.tsx` + scoped `RequisitionForm.css`, shared by create + edit; matches
  `resources/ProQ-new-requisition.html`, see `docs/plans/16-proq-requisition-form.md`): a **5-step**
  flow (Requester details / Procurement Category / Requirement Details / Vendor & budget / Review),
  styled with the ProQ visual language (dark hero, pill stepper, Space Grotesk/IBM Plex Mono/Inter
  fonts loaded in `index.html`) scoped under `.proq-req` so it doesn't leak into the Tailwind app.
  There are **three categories** (`PRCategory` = `IT | NON-IT | EVENTS`; `category` is free-text on
  `purchase_requests` — no CHECK/enum, so EVENTS needed no migration); **Marketing & Events** is a
  real stored category rendered as "under development" placeholders. The rest of the form is a
  `details` **JSONB** blob (shape = `PRDetails`). Following the ProQ mockup, the form **no longer
  collects** WSO2 entity, estimated value, or currency — those
  columns remain (nullable/defaulted) and pass through unchanged when editing legacy PRs, but new PRs
  leave them empty. Budget coding (category / product / region / engagement code) is stored as strings
  in `details`, but the **values are picked from the Settings-page config lists**
  (`budget_category`/`product`/`region`/`engagement_code` via `useConfigLookup` + `optionsFor`): the
  shared `OptionField` helper in `RequisitionForm.tsx` renders a `<select>` for a list of ≤10 values, a
  type-to-search combobox (`OptionCombobox`) once it exceeds 10, and a plain free-text input when the
  list is empty (as `engagement_code` ships) — free text is still accepted so pre-existing/unlisted
  values survive. The form **does** collect a **business unit** (dropdown, required) and a
  **budget approver** (dropdown, required) chosen from that unit's approvers — see the **Business
  units** entry below. `team_lead_email` is also required. The selected approver is stored in the
  free-text `budget_approver_name/email`; the recommendation's **budget approval** matches the caller's
  email against `budget_approver_email` (case-insensitive) — see
  `IsBudgetApproverForPR`/`BudgetApproversForPR` in `repository/recommendations.go` and the
  `budgetEmailMatch` fragment folded into `approvablePredicate`/`myApprovalStateExpr` in
  `repository.go`. Supplier attachments staged in the create form are uploaded (best-effort) after the
  PR is created. Old `pr_items`/`pr_links` are unused (kept for legacy rows).
- **Phase 2 (procurement view)** — see `docs/plans/02-finance-view.md` (plan archive) and
  `docs/contracts.md` (as-built contract model). The procurement workflow up to contract signing:
  vendors, quotations (with line items), and contracts. **RFQs were removed** (migration `021`): a PR
  now has zero or more quotations attached **directly** (`quotations.purchase_request_id`), and
  procurement users associate a quotation inline on the PR page (vendor + optional description + the
  initial quotation PDF) without leaving it. The quotation detail page still carries the richer fields
  (line items, total, currency, valid-until). A quotation has **two primary PDFs — an "initial
  quotation" and a "final quotation"** — plus **zero or more other documents** (migrations `036` +
  `048`, `quotations.initial_quotation_document_id`/`final_quotation_document_id` FKs → `documents`,
  mirroring `contracts.signed_document_id`): both primaries are pulled out of the doc list on reads
  (`initial_quotation_document`/`final_quotation_document` vs `documents`) and are each managed at
  `.../quotations/{id}/{initial,final}-quotation-document` (upload replaces; delete removes — repo
  `SetQuotationDocument`/`ClearQuotationDocument` take a `QuotationDocSlot`), while other docs use
  `.../quotations/{id}/documents`. **Only the initial** PDF can be attached while creating a
  quotation; the **final** one is uploaded afterwards from the quotation's card, is optional, and
  **never gates selecting** the quotation for the recommendation. On the PR page each quotation is a
  **card** (`QuotationCard`) showing vendor, description and both PDF slots inline — each slot with an
  Upload/Replace + Remove control (`QuotationPdfSlotRow`) **plus that PDF's own extracted
  total/validity/line items** (see the extraction entry below). The card-level "More details"
  expander (and its lazy `GetQuotation` fetch) **was removed**: the initial and final PDFs
  legitimately disagree, so one card-level summary could only ever show one of them; the card
  header now links to `/quotations/{id}` for the stored record and any other documents. The two
  primaries ride along on
  the *summary* reads (`quotationSummarySelect` LEFT JOINs them) so the cards need no detail fetch.
  Selecting a quotation advances the PR to `vendor_selected`; the contract
  itself is created and managed from the recommendation's **contract card** (below). **Contract
  review/approval was removed** (migration `032`): a contract is simply `draft` until its **signed
  PDF** is attached, then `signed` — approvals live only on the PR (the recommendation cards). A
  contract now holds one or more **draft-contract PDFs** plus a single **signed PDF**, each with its
  own `notes` (`documents.notes`, migration `031`); see `docs/contracts.md`. Any
  `procurement`/`procurement_admin`/`admin` user performs all procurement actions (helper
  `middleware.HasProcurementAccess`); `staff` is unchanged. Statuses auto-advance via
  `model/transitions.go` (the first quotation moves a PR `submitted → under_review`; PR terminal
  state for this phase is `order_signed`; `completed` is reserved for the future payment phase). The
  `documents` table is generalized with `owner_type`/`owner_id` so quotations/contracts share it
  while files still live under the owning PR's storage dir. NetSuite PO / payment settlement remain
  out of scope.
- **Phase 3 (contract fulfillment)** — see `docs/plans/03-invoices-grns.md`. Once a contract is
  `signed`, procurement users record one or more **GRNs** (goods received notes) and **invoices**
  against it — both with line items and their own documents (shared `documents` table, owner
  types `grn`/`invoice`). Invoices walk `received → approved → paid` (revertible one step each
  way) and are editable/deletable only while `received`. An invoice total may be **entered
  directly** (`invoices.entered_total`, migration `020`), which takes priority; otherwise the
  stored `total_amount` is derived server-side from line items. When both are present and differ,
  the UI shows a non-blocking mismatch **warning** and the entered total wins. The contract
  exposes `invoiced_total` to drive an over-billing **warning** (not a block). The PR stays at `order_signed` — `completed` remains reserved for a future
  payment phase. New entities are contract-scoped (not in the PR-anchored case graph); create is
  nested under `/contracts/{id}`, entity ops live at `/grns/{id}` and `/invoices/{id}`.
- **Procurement recommendation** — migration `022`; backend in
  `repository/recommendations.go` + `handler/recommendations.go`. Once a PR has ≥1 quotation, procurement
  adds a **single** recommendation (vendor — restricted to a quoted vendor — + description + the
  required approvals: budget owner / legal / security / compliance, budget default-checked) inline on
  the PR page.
  Each required approval is an **approval card** with an approve toggle and a comment thread (text +
  doc attachments, owner type `pr_recommendation_comment`). Card actors: each **team card** maps to its
  role via `model.RecTeamApprovalRoles` (legal→`legal`, security→`security`, compliance→`compliance` —
  adding a team card is a one-line change there plus a seeded `teams` row);
  budget→the PR's **named budget approver** (the person the requester picked
  from the business unit's approvers, stored in `budget_approver_email`): `IsBudgetApproverForPR` /
  `BudgetApproversForPR` match the caller's email against it (case-insensitive); admin passes any.
  The card a caller may act on is server-computed
  into `recommendation.my_actionable_types` (comment/view) plus per-card `can_comment`/`can_approve`/
  `can_assign` flags — see the **Teams & approval assignees** entry below for the team-card
  assignee split (comment = whole team, approve = the assignee only). Caller-side card membership is
  resolved once by `handler.callerRecCardTypes` and passed to the repository as the `text[]` `$2` bind
  of `approvablePredicate`/`myApprovalStateExpr` (`$3` = caller email).
  **All required cards must be approved before a quotation can be selected**
  (`RecommendationFullyApproved`, enforced in `SelectQuotation`). Editing the recommendation resets all cards to pending. PR view/list
  access is extended so card actors can see and find the PRs awaiting them. The recommendation view
  renders each item as an elevated **sub-card** (`components/RecommendationSection.tsx` →
  `SubCard`/`IconTile`): the approval cards, plus two optional cards procurement manages inline:
  - **Budget approval chain** (migration `045`; `docs/chained-budget-approvals.md`): the budget card
    is a **serial chain of steps** (`pr_recommendation_budget_steps`), not a single toggle. Step 1
    (base) is governed by the PR's named budget approver (above); procurement appends further **named**
    steps (approver email, matched case-insensitively like team-lead) via a "+ Add another approval
    step" control. Each
    step is **approve / reject / pending**; a step is decidable only once the previous is approved, and
    **locks once the next step decides** (approve/reject/reverse until then). The
    `pr_recommendation_approvals` budget row's `approved_by` is now a **projection** — set iff every
    step is approved (`syncBudgetApprovalFromStepsTx`), so `recApprovedSQL`/`SelectQuotation` are
    unchanged; a rejected/pending step blocks selection. Endpoints under
    `.../recommendation/budget-steps[/{stepID}[/decision|/remind]]`; budget decisions no longer use
    `POST .../approvals/budget` (it 400s). Per-step `can_decide`/`can_manage` flags; step comments carry
    `budget_step_id`. A named step approver (not the base budget approver) gets view/list access via
    `callerCanView` + the budget-step branch of `approvablePredicate`/`myApprovalStateExpr`/
    `IsApproverForPR` (which gained a `callerEmail` arg — `$4` = caller email). Process events:
    `rec_approval_budget` (approve/reject/revert) for decisions, `update_budget_chain` (add/update/remove)
    for chain edits.
  - **RFI card** (migration `030`; `rfi_description` on `pr_recommendations` + docs owner type
    `pr_recommendation_rfi`): a request-for-information raised with the vendor — a description plus one
    or more PDF attachments. Endpoints under `.../recommendation/rfi`.
  - **Contract card** (migration `029`; `pr_recommendations.contract_id`): creates/links a real
    contract (so it appears on the Contracts page) via `CreateRecommendationContract` — **ungated**.
    This is now the **only** way to create a contract; the old quotation→contract path
    (`POST /quotations/{id}/contracts`, `CreateContractFromQuotation`) and its quotation-page
    "Resulting contracts" card were removed — contracts are created only from the PR page. It embeds
    the shared `ContractContent` (draft-contract PDFs + signed
    PDF, each with notes; see `docs/contracts.md`). The whole contract can be removed from the card
    while still `draft` (`DeleteRecommendationContract`, which rewinds the PR to `vendor_selected`).
- **Purchase-request approvals** — see `docs/plans/05-pr-approvals.md` and
  `docs/pr-approvals.md`. A PR names one or more approvers at creation (required,
  ≥1, anyone but the requester); each approves/rejects with a comment, and the
  requester re-requests a rejected one. Approvals are **informational** (no longer
  gate quotations — the RFQ gate was removed with RFQs). Per-approver rows live in
  `pr_approvals` (migration `017`),
  separate from PR `status` — the same per-actor approve/reject shape used by the
  recommendation approval cards. Optional
  `email:` config drives notifications (`internal/email`; logs in dev when
  disabled). Approver lookup: `GET /users/lookup` (any authenticated user).
- **Team lead approval** — see `docs/team-lead-approval.md` (migration `037`). *Separate* from the
  named-approver system above and **mandatory**: every PR names a **team lead by email** (required
  field below business justification on the requisition form) who must approve it before procurement can
  act. Decision state lives on `purchase_requests` (`team_lead_email/status/notes/decided_at/decided_by`;
  existing rows grandfathered to `approved`). Team-lead identity is a **case-insensitive email match**
  (works before the person has logged in). **Visibility gate**: until `team_lead_status='approved'`,
  the PR is visible only to the requester, the team lead, and admins — enforced in
  `handler.callerCanView` and `repository.ListPurchaseRequests`/`HasApprovableWork`. **Explicit gate**:
  quotation-create and recommendation-create also 409 until approved. Decision at
  `POST .../team-lead-approval` (`{decision, notes}`; notes required on reject; team lead or admin only;
  revisable — "Edit decision"); process event `team_lead_approval`. On **approve**, the Procurement team's
  shared email (`teams.team_email` where `member_role='procurement'`) is notified that the PR is ready for
  procurement (`notifyProcurementTeamOfApproval`, best-effort; no-op if that email is unset). Editing a PR (or changing the
  team-lead email) **resets a rejected/changed decision to pending** (edit & resubmit). UI: a dedicated
  `TeamLeadApprovalCard` with a `ConfirmDialog` modal; the team lead sees the PR under **Approvals**.
- **PR assignment** — see `docs/pr-assignment.md` (migration `044`). *After* team-lead approval and
  *before* any procurement work, a PR must be **assigned** to a member of the procurement team. A PR
  carries a single **assignee** (`purchase_requests.assignee_id/assigned_at/assigned_by`) plus zero or
  more **collaborators** (`purchase_request_collaborators` join table); the assignee is the owner and
  collaborators are the procurement users who have also worked on it (auto-tracked, see below). **Assign
  rules**: a `procurement` user self-assigns
  (claims an unassigned PR) or hands back their own; `procurement_admin`/`admin` assign/reassign anyone
  (dropdown of procurement-team members + themselves) and manage collaborators (so does the assignee).
  Valid assignees/collaborators must hold `procurement`, `procurement_admin`, or `admin`
  (`repository.SetPRAssignee`/`AddPRCollaborator` via `CanBeAssignedPR`; `ErrNotProcurementUser`) — so a
  `procurement_admin` can assign a PR to themselves even without the plain `procurement` role. **Work gate**
  (`handler.assignmentWorkGate`, alongside the team-lead 409): quotation-create and recommendation-create
  return **409** while unassigned. Assignment tracks the owner but **does not lock others out** — once
  assigned, *any* `procurement`/`procurement_admin` user may work on the PR, and doing non-readonly work
  **auto-enrolls them as a collaborator** (`handler.ensureCollaborator` → `repository.EnsurePRCollaborator`,
  best-effort; skips the assignee/existing collaborators and non-procurement actors; wired into the
  quotation and recommendation/contract/RFI/budget-approver writes). Endpoints
  `PUT .../assignee` (`{assignee_id}`, null to unassign), `POST/DELETE .../collaborators[/{userID}]`;
  process events `assign_pr` (qualifier `assign`/`unassign`) and `update_pr_collaborators`. Per-caller
  flags `my_can_assign`/`my_can_manage_collaborators`/`my_can_work` on PR reads
  (`handler.attachAssignmentActionable`) drive the UI: a dedicated `AssignmentCard` (dropdown or
  "Assign to me"/"Unassign me" + `ApproverPicker` for collaborators) between the team-lead card and the
  procurement sections; `ProcurementSection`/recommendation authoring are gated on `my_can_work`. The
  list page shows an **Assignee** column ("Assigned to you" / assignee / amber "Unassigned").
  The same card carries the PR's **priority** (migration `051`): `purchase_requests.priority`, a
  `P1` (red, high) / `P2` (yellow) / `P3` (green, **default**) triage level with a DB CHECK, so every
  existing row starts at P3 and the requisition form never collects it. Any
  procurement/procurement_admin (and admin) user sets it via `PUT .../priority` (`{priority}` →
  `repository.SetPRPriority`; 403 non-procurement, 409 before team-lead approval, 400 on an unknown
  value); unlike procurement *work* it does **not** require the PR to be assigned — that's how an
  unassigned PR gets triaged. Per-caller flag `my_can_set_priority`
  (`attachAssignmentActionable`) switches the card between a coloured `<select>` and a read-only
  chip; the list page shows a **Priority** column. Process event `update_pr_priority` with the new
  level as the qualifier. Frontend `PRPriority`/`PR_PRIORITIES`/`prPriority`/`prPriorityColor` in
  `types/api.ts` are the one place the levels and their colours are defined.
- **Sessions (60-day sign-ins)** — see `docs/sessions.md` (migration `052`). Ported from
  `../finance-apps`. How long a user stays signed in is the **app's** decision, not the IdP's: the
  SPA logs in through Asgardeo exactly as before, exchanges that token **once** at
  `POST /api/v1/auth/session` for an **HttpOnly cookie**, and every later request authenticates with
  the cookie — `middleware.Authenticate` checks it **before** the `Authorization` header, so a stale
  token the browser still holds is ignored rather than rejected. `user_sessions` stores only the
  **SHA-256 hash** of a 256-bit token; `Repository.TouchUserSession` resolves it *and* slides
  `expires_at` forward in one statement, so 60 days means 60 days **idle** (`GREATEST`, so shortening
  the TTL never extends existing rows; `last_used_at` only rewritten when >1min stale). Backend in
  `middleware/session.go` + `repository/user_sessions.go` + `handler/auth_session.go`;
  `DELETE .../auth/session` signs out **server-side** (a cleared cookie alone would leave a captured
  one working) and `DELETE .../auth/sessions` is sign-out-everywhere — the containment lever that
  makes a long-lived cookie safe. Cookie-authenticated **writes** are CSRF-guarded twice (SameSite +
  `CheckCSRFOrigin` against `cors.allowed_origins`); the bootstrap-admin grant and deactivation gate
  run on the cookie path too (`serveAuthenticated`), and `pruneSessions` deletes dead rows daily.
  Config is a new **`session:`** block (`enabled` default **true**, `ttl_days` 60, `max_days` 90,
  `cookie_secure` true — set **false** for dev over http://localhost, `cookie_samesite` `lax`|`none`,
  where `none` implies Secure + Partitioned); `enabled: false` is the rollback to Bearer-only.
  Because the IdP is **not consulted after mint**, three things bound/close that gap:
  an **absolute cap** (`max_days`, enforced in the same `TouchUserSession` statement and in the
  purge; `0` disables it and is warned about at startup), an **admin kill switch**
  (`DELETE /users/{id}/sessions` → Users page "End sessions" button, admin-only, audit
  `revoke_user_sessions`/`admin`; 404 on an unknown id), and **OIDC back-channel logout**
  (`POST /auth/backchannel-logout` — the **only route outside the authenticated group**, since the
  signed logout token *is* the credential: `middleware.VerifyLogoutToken` in `logout_token.go`
  checks signature/iss/aud/`events`/no-`nonce`/`iat` freshness, deliberately **not** reusing
  `provider.Verifier`, which would accept an ID token as a logout instruction). Revocation prefers
  the token's `sid` — captured from the ID token into `user_sessions.idp_sid` at mint time
  (migration `053`), so one browser is ended rather than all — and falls back to `sub`; it returns
  **200 even when nothing matched** (the IdP broadcasts to every app) and `400`, detail-free, on an
  unverifiable token. Front-channel logout was rejected: it only fires with a tab open, missing the
  offboarding case. Because Asgardeo's console exposes no logout-URL field for **SPA-template** apps
  (a pure SPA has no server to call back; this one does), a fourth, pull-based mechanism closes the
  case that actually matters — **`internal/offboard`**, the IdP offboarding sweep: every
  `session.idp_offboarding.interval_minutes` (default 10) it compares the users holding live sessions
  (`UsersWithLiveSessions`) against the **SCIM directory snapshot the pickers already cache**
  (`forceRefresh=false`, so no extra SCIM traffic) and revokes the sessions of anyone **absent**
  (deleted) or **`active: false`** (disabled at the IdP), matched by lowercased email. It revokes
  **sessions only, never deactivating the app user** (revocation is self-correcting; auto-deactivation
  would need auto-reactivation and would fight an admin). It **requires `scim.enabled`** — the DB
  fallback would make "missing" self-referential — and `Decide` (pure, unit-tested) refuses to act on
  an untrustworthy snapshot: empty, no usable emails, or a run that would sign out >50% of signed-in
  users once ≥5 are involved. `dry_run` logs without revoking; each revocation audits
  `revoke_user_sessions`/`idp_offboard` with a **NULL actor**. It does *not* catch a plain IdP
  sign-out — no directory trace — which is what back-channel logout is for. In-app
  **role/deactivation** changes need none of this — both are re-read per request. Frontend:
  `api/session.ts`, `apiCredentials()` in `api/client.ts` (cookie **on** for a same-origin/same-host
  API, **off** cross-site unless `config.js` sets `apiAllowCredentials` — `credentials: 'include'`
  makes the browser *require* `Access-Control-Allow-Credentials`, so enabling it against an
  unprepared gateway breaks every request), a 401 → `session-expired` event → login page with an
  "expired" notice, and `AuthContext` boots **cookie → token-in-this-tab → login**. Signed-in state
  is `authenticated`, **not** the OIDC user (the steady-state tab holds no live token);
  `automaticSilentRenew` is **off** and token expiry is no longer a logout — that was the cause of the
  daily sign-outs. Same-origin `/api` (below) is what makes the cookie first-party. No new event
  action (the app does not audit logins).
- **Frontend deployment (Dockerfile build)** — see
  `docs/deployment-guide.md` → "Frontend — Choreo web app". The Choreo web app component is built
  from **`webapp/Dockerfile`** (`node:20-alpine` → `npm ci && npm run build`, then
  `nginxinc/nginx-unprivileged:1.30-alpine` serving `dist/` on **8080** as UID **10014**), *not* the
  static-web-app buildpack — which serves `dist/` from its own server and **ignores
  `webapp/public/nginx.conf`**, so the security headers were never sent and no proxy was possible.
  Our nginx adds both: the header set (CSP / frame / referrer / HSTS / permissions) and a **`/api`
  reverse proxy** to the backend, which makes the API **same-origin** so the session cookie is a
  first-party `SameSite=Lax` one (works in Safari; needs no gateway credentials change) — hence
  `apiBaseUrl: ''` in the mounted `config.js` and `session.cookie_samesite: "lax"`. The proxy target
  is inlined in `nginx.conf` (`set $backend …`), so changing environment means a rebuild;
  `client_max_body_size 32m` covers the 20MB PDF cap and the read timeout outwaits the 180s
  extraction. The **CSP only becomes real on this build**, so it enumerates what the app actually
  loads: Google Fonts (`fonts.googleapis.com`/`gstatic`) from `index.html`, Asgardeo, and the
  `apis.google.com`/`accounts.google.com`/`docs.google.com` scripts + iframe behind
  **Settings → File storage** (Drive Picker) — a blocked script there fails silently, so re-verify
  that flow after any CSP edit. `.dockerignore` keeps host `node_modules`/`dist`/`tsbuildinfo` out of
  the build, and the Dockerfile deletes the `nginx.conf` copy Vite puts in `dist/`. **Choreo scans the
  final image with Trivy and fails the build on any CRITICAL OS-package finding** — hence the current
  base tag (the `1.27-alpine`/Alpine 3.21 line fails on openssl CVE-2026-31789) plus
  `apk upgrade --no-cache libssl3 libcrypto3` so a lagging base can't reintroduce it. On a future
  failure, bump the tag — don't add an ignore; scan locally with `aquasec/trivy` first
  (command in `docs/deployment-guide.md`).
- **User management (admin)** — see `docs/user-management.md`. Admins add users *by email* (pending
  invites — `users.sub` is now nullable, claimed on first login by email match), grant/revoke roles
  (`staff` is a non-removable baseline), and deactivate users (`users.is_active`; blocks login). An
  invited user's email/name can be **edited** (`PUT /users/{id}`, `UpdateInvitedUser`, audit
  `update_user`) **only while pending** — once they've signed in, `sub` is set and the email is the
  login-match key (409 `ErrUserAlreadyLoggedIn`). A last-admin guard prevents lockout. UI at `/users`
  (admin-only nav link); API under `/api/v1/users`.
- **Audit & process events** — see `docs/process-events.md`. Two append-only tables (migrations
  `034`/`035`): `process_events` logs the significant PR business-process tasks (BPMN-style —
  `submit_pr`, `pr_approval`, `add_quotation`, `sign_contract`, …), `audit_events` logs non-process
  master-data/admin mutations (vendors, business units, config options, users/roles, storage). The
  `action` set is fixed in Go (`model.ValidProcessActions`/`ValidAuditActions`, in
  `internal/model/events.go`) and validated on write — a task-decision direction (approve/reject,
  sign/unsign, target status) is a `qualifier`, not a new action. Recorded best-effort at the handler
  layer (`recordProcessEvent`/`recordAuditEvent`) after a successful mutation; PR status
  auto-advances are *not* recorded (derivable). A read view (issue #2502) for **admin /
  procurement_admin** exposes both logs at `/audit` ("Audit log" nav): one page, a segmented control
  over **Process events** and **System events**, filtered server-side by actor / action / date range
  (+ PR id for process). Backend `Repository.FilterProcessEvents`/`FilterAuditEvents` (LEFT JOIN users
  for a display `actor_name`) + `EventsHandler` (`handler/events_view.go`) at
  `GET /api/v1/events/{process,audit,actions}`, gated by `requireAccess` (`HasRole(admin) ||
  HasRole(procurement_admin)`; frontend `useCanViewAuditLog`/`canViewAuditLog` nav flag); capped at
  the latest 2000 rows/section. Read-only — no new event action.
- **BPM analytics** — see `docs/bpm-analytics.md`. An **Analytics** nav group (sidebar, directly
  above **Admin**; a group, not a single item, because more subsections are expected) over the
  process data the app already records. Read-only: no migration, no new event action, no writes.
  Phase 1 is one subsection, **Purchase requests**: the list of every PR (id/reference, title,
  priority, created, requester, assignee) and, per PR, its **process-event flow** drawn top-to-bottom.
  Gated to **admin/procurement_admin** by the *same* function as the audit log —
  `handler.hasEventLogAccess`, extracted from `EventsHandler.requireAccess` and shared — because the
  flow view returns the **same `process_events` rows**; frontend `useCanViewAnalytics` +
  `canViewAnalytics` nav flag. Deliberately applies **no visibility gate**: `repository/analytics.go`
  takes no caller args at all, so the list includes PRs still awaiting team-lead approval and the flow
  read bypasses the gated PR read (a `procurement_admin` can analyse a request they can't open) —
  sound only because that audience already sees every PR's events in the audit log. Backend
  `ListAnalyticsPRs`/`GetAnalyticsPR` + `handler/analytics.go` at
  `GET /api/v1/analytics/purchase-requests[/{id}]` (the detail returns header **and** events in one
  round trip). **Sorting is server-side** (`?sort=created_at|requester|assignee&dir=asc|desc`, unknown
  values falling back rather than erroring, each column defaulting to its natural direction) because
  the list is capped at 500/2000 — sorting the page locally would reorder a slice of the data and call
  it an ordering; `analyticsPROrderBy` resolves the column through a `switch` (never interpolated),
  sorts people on the rendered `COALESCE(NULLIF(name,''), email)` string, keeps assignee `NULLS LAST`
  in **both** directions, and breaks every tie on `id`. `Repository.ListProcessEvents` (the per-PR
  oldest-first read) gained the `users` LEFT JOIN for `actor_name` and stays **unbounded** — one
  request's own history, and a truncated flow diagram would misstate the process. Frontend
  `components/ProcessFlowTimeline.tsx`: one continuous rail, one node per event, and the **elapsed
  time on the connector** between two nodes (the wait is a property of the gap, not of either step).
  Colour carries **outcome only**, from the reserved status roles and derived from the *qualifier*
  (`eventOutcome` in the new shared `lib/eventLabels.ts`, which also holds the `humanizeEventToken`
  the Audit log page previously kept a local copy of); tiles are outlined not filled so the glyph
  keeps contrast in both themes, and every node pairs colour with an icon **and** the visible
  qualifier chip, so nothing is colour-alone.
- **Teams & approval assignees** — see `docs/teams-approval-assignee.md` (migrations `038` teams,
  `039` assignee, `047` the Compliance team + `compliance` role). A **team** (`teams` table, seeded
  fixed set Legal/Security/Compliance/Procurement) is a name
  + a fixed/display-only `member_role` + a shared `team_email`.
  Membership **is** the role — there is no membership table; adding/removing a member grants/revokes
  `member_role` (reusing `EnsureUserHasRole`/`RemoveUserRole`). A team may also have an **admin-role
  variant** that counts as part of the team (currently only Procurement → `procurement_admin`, via
  `adminRoleForMemberRole`): those holders are surfaced separately as `Team.admin_members` while
  `Team.members` stays scoped to plain `member_role` holders (so the assignee pool keyed on it is
  unchanged); admins are managed from the Users page, and the Teams card lists them badged/non-removable.
  Managed in a **Teams** section on the
  Settings page (`components/TeamsSection.tsx`); members + email are procurement_admin/admin
  (`middleware.HasTeamAdmin`), reading is open (`GET /teams`, needed by the assignee dropdown). The
  team recommendation approval cards (legal/security/compliance) gain an **assignee**
  (`pr_recommendation_approvals.assignee_id`): the previously all-or-nothing card is split three ways,
  serialized as `can_comment`/`can_approve`/`can_assign` — **comment** = any team member (as before),
  **approve** = **the assignee only** (`canApproveRecType`), **assign** = any team member or procurement
  (`canAssignRecType`); budget card unchanged. Setting the assignee prompts to notify (email + link,
  CC the `team_email`); a **Send reminder** button re-sends. Email uses the existing SMTP mailer
  (`Mailer.SendCC` added for the CC); disabled in dev = logged. Endpoints
  `PUT/POST .../recommendation/approvals/{type}/assignee[/remind]`; process events
  `assign_rec_legal`/`assign_rec_security`/`assign_rec_compliance` (qualifier `assign`/`unassign`).
  **Compliance** (migration `047`) is a straight clone of Security: a `compliance` role + seeded team,
  a `compliance` approval card (optional, assignee-approved, gates quotation selection like the
  others), and process events `rec_approval_compliance`/`assign_rec_compliance`.
- **Business units (was budget units, was cost centers)** — see `docs/business-units.md` (migration
  `046`, which renames `budget_units` → **`business_units`**, drops the bracket/currency/value model,
  and — dev-only, no data migration — **truncates** existing data). A business unit is just a **name**,
  an optional **description**, and a **flat list of approvers** (`business_unit_approvers`) — no
  brackets, no unit budget/currency, no default approver, and the `resolve_budget_approvers` SQL
  function is gone. On the **requisition form** the requester picks a business unit (dropdown, required)
  and then a **budget approver** from that unit's approvers (dropdown, required); the choice is stored in
  the free-text `purchase_requests.budget_approver_name/email`. Budget-approval authority everywhere is a
  case-insensitive **email match** against `budget_approver_email` — `IsBudgetApproverForPR` /
  `BudgetApproversForPR` in `repository/recommendations.go` and the `budgetEmailMatch` fragment in
  `approvablePredicate`/`myApprovalStateExpr` (`repository.go`). `purchase_requests.budget_unit_id` →
  `business_unit_id`; `invoice_cost_allocations.budget_unit_id` → `business_unit_id`. Management
  (create/edit, deactivate) is procurement_admin/admin at `/business-units`
  (`middleware.HasBusinessUnitAdmin`, which also gates config options); the active-unit **lookup**
  (`GET /business-units/lookup`) and the unit's **approver list** (`GET /business-units/{id}/approvers`,
  which feeds the budget-approver dropdown) are open to any authenticated user. Procurement can re-notify
  the budget approver via `POST .../recommendation/approvals/budget/remind` (`remindBudgetApprovers`);
  the approver is also emailed on recommendation create/update. Audit actions
  `create_business_unit`/`update_business_unit`, entity `business_unit`. The recommendation budget card
  shows the named approver — procurement changes it inline (dropdown of the unit's approvers) via
  `PUT /purchase-requests/{id}/budget-approver` (`SetBudgetApprover`, procurement; touches only the
  approver fields).
- **Role-based home page** — see `docs/home-pages.md` and `docs/plans/18-role-home-pages.md`. The
  post-login landing page (`/`, first nav item "Home") is a dashboard of count tiles + a "latest
  activity" feed, rendered as **stacked sections per role** the caller has (mirrors the nav): **My
  requests** (everyone), **Approvals** (when `is_approver`), **Procurement** (procurement/admin).
  One read-only endpoint `GET /api/v1/home` (`handler/home.go`) returns `HomeResponse{staff?,
  approvals?, procurement?}`; aggregation in `repository/home.go` **reuses** the `approvablePredicate`/
  `myApprovalStateExpr`/`teamLeadMatch`/`budgetEmailMatch` fragments (same `$1..$4` bind order as
  `ListPurchaseRequests`). "Completed" = `order_signed` (+ reserved `completed`); the procurement
  card splits `order_signed` into **awaiting delivery** (not fully paid) vs **completed** (≥1 invoice,
  all `paid`, via `invoices.purchase_request_id`/`status`). No migration, no new event action (read).
- **Quotation PDF extraction (Claude)** — see `docs/quotation-extraction.md` and
  `docs/plans/20-quotation-pdf-extraction.md` (migration `049`). A quotation PDF is run
  through the Anthropic API to read vendor / currency / **totals block** (subtotal, discount,
  every tax line, shipping & other charges, grand total, tax-inclusive flag) / validity /
  line items, and the result is offered as **editable suggestions** a procurement user confirms — nothing
  is written onto a quotation automatically. Backend `internal/extraction` mirrors
  `internal/directory`: config-gated by a new **`anthropic:`** block (`enabled: false`
  default, so dev/CI need no key; the key is server-side only), one *streaming* call per
  PDF using `claude-opus-5` + adaptive thinking + `output_config.format` (JSON schema, all
  fields nullable), with the stable system prompt as the cached prefix. Results stage in
  **`quotation_extractions`** (`raw_json`, token counts, `pending|succeeded|failed`,
  `applied_at/by`; unique partial index on `document_id` where not failed ⇒ re-running on a
  PDF replaces the live row while failures are kept). Two flows share it all: **pre-create**
  (the primary one — drop a PDF on the PR page, review the pre-filled form, and one
  `CreateQuotation` carries `extraction_id`, which *adopts* the already-stored document into
  the initial-PDF slot via a new `SetDocumentOwner` + the existing `SetQuotationDocument`,
  so bytes are never uploaded twice — the staging doc's owner type is
  `quotation_extraction`) and **post-create** ("Read details" per PDF slot on the quotation
  card → Apply → `UpdateQuotation`). Trust rules are the design: the model returns a vendor
  **name**, never an id (`MatchVendors` ranks `vendors` 100/80/60/40 after normalising case,
  punctuation and legal forms; the UI preselects only at ≥80, and when *nothing* ranks
  `VendorSelect` opens its new-vendor form prefilled with the extracted name — new optional
  `suggestedName`/`defaultAdding` props — so creating it is one click, with a caption warning
  the name came from the PDF); omitted fields stay nil so a
  missing total shows blank, never `0`; the total is cross-checked against the line items
  with the same non-blocking warning invoices use for `entered_total`; **split taxes stay
  split** (`taxes` is a list — CGST+SGST / ICMS+PIS+COFINS are never summed) and "no tax
  stated" is an empty list + null `total_includes_tax`, never a zero; `stop_reason` is
  checked before reading content so refusals/truncation surface as messages. Results are
  displayed **per document, not per quotation**: the initial and final PDF are separate
  documents whose figures and line items legitimately differ, so each PDF slot on the PR
  page carries its own collapsible "Read from this PDF" panel
  (`components/ExtractedQuotationDetails.tsx`, sharing its line-item table and totals block
  with the editable `QuotationExtractionReview.tsx`) with an `applied`/`not applied` chip
  showing whose numbers the quotation actually holds — stamped by **both** apply paths
  (create via `adoptExtractionDocument`, update via `markExtractionApplied` when the body
  carries an `extraction_id`; the latter also rejects another PR's extraction), and
  **exclusive per quotation** (`MarkExtractionApplied` clears the stamp on the quotation's
  other extractions, since one stored total can only come from one PDF). Both stamps are
  best-effort — the quotation write already succeeded. The **tax breakdown lives with the
  extraction, not on `quotations`** (which stores one total/currency/validity/items) — those
  figures are a property of a document and the two documents disagree, so adding columns
  would force one to win. The single stored total is **always tax-inclusive**:
  `lib/extractionTotals.ts` (pure, shared by both panels so shown/warned/saved can't drift)
  adds tax when the PDF says the total excludes it *or* when the printed total equals a
  pre-tax base, derives the total from the breakdown when none is printed, and otherwise
  leaves an explicit grand total alone; any adjustment is stated in words under the field.
  The line-item cross-check is made at the same level (`reconciledTotal` = items − discount
  + taxes + charges), so a taxed quotation isn't flagged merely for being taxed. **Uploading
  a PDF into a card slot reads it immediately** and opens the editable review inside that
  slot; "Read details" disables itself once a PDF has been read (replacing the PDF makes a
  new document, so it re-enables). Endpoints
  `GET /quotation-extractions/{status,{id}}`, `POST /purchase-requests/{id}/quotation-extractions`,
  `GET /purchase-requests/{id}/quotation-extractions` (succeeded rows, one per document,
  procurement-gated like `ListForPR`, no vendor ranking; frontend `usePRExtractions` fetches
  it once per PR and cards look up their slots by `document_id`),
  `POST /quotations/{id}/extract?slot=initial|final`, gated like all procurement work
  (`HasProcurementAccess` + team-lead 409 + `assignmentWorkGate`); process event
  `extract_quotation` (qualifier `initial|final|staged`). Frontend gate
  `useCanExtractQuotations` (feature hidden when unconfigured). Schema gotcha, verified live:
  a property may not carry **both `enum` and a union `type`** — the validator checks each enum
  value against a single declared type and 400s, so `confidence` is `enum`-only with `null` as
  a member (`TestSchemaEnumsCarryNoType` guards it; `live_schema_test.go` is the opt-in live
  contract check via `PURCHASING_LIVE_ANTHROPIC_KEY`). The schema and a synthetic quotation
  are verified against the real API; **accuracy against real PDFs is still untested** —
  `resources/files/` holds four sample PDFs as a first eval set (one is Portuguese/BRL).
- **Quotation comparison** — see `docs/quotation-comparison.md` (migration `050`). The app's
  version of `resources/files/WSO2_Vendor_Quote_Comparison_Template.xlsx`: every vendor's
  **initial quote beside its final (post-negotiation) quote**, with the variances and savings that
  follow. Procurement gets a **"Do quotation comparison"** button in the Quotations card header
  once a PR has **≥2** quotations; the card (`components/QuotationComparisonCard.tsx`) renders at
  **page** level between the procurement section and the recommendation, because the people it
  exists for — the recommendation's approvers — have no procurement access and so could never
  assemble it client-side. **Nothing is snapshotted**: `quotation_comparisons` (one row per PR)
  stores only `generated_at/by`, the **approved budget** (the sheet's header field — the
  requisition form no longer collects an estimated value, so procurement types it on the card),
  the `currency`, and `use_initial_for_final`; every figure is assembled per read by
  `assembleComparison` (`handler/comparisons.go`) from the quotations + their per-document
  extractions, so **a quotation edited after generating updates the comparison** with no
  mechanism (5s poll, plus the PR page invalidating `["quotation-comparison", prId]` on every
  quotation write). Column sources, in precedence order: `pdf` (that slot's own extraction —
  split taxes stay split), `record` (the quotation row's stored total/items — the only source
  when extraction is unconfigured, and *not* used for the initial column when the stored figures
  are the final PDF's), `initial` (the stand-in below). A `0`-total placeholder quotation carries
  **no** figures, never `0.00`. Totals are tax-inclusive via `extraction.TaxInclusiveTotal` — the
  **Go twin of `lib/extractionTotals.ts`**, which must stay in step (both unit-tested over the
  same cases) or one PDF would show two totals on one page. **Missing final quotes**: the modal
  names the vendors (`missing_final` is computed whether or not a comparison exists) and
  confirming posts `use_initial_for_final`, which the server also enforces (409 naming them);
  those columns are labelled "initial figures — final quotation not available", stay flagged, and
  switch to the real numbers as soon as a final PDF is read. Layout for a card rather than a
  spreadsheet: one **summary matrix** (metrics × vendors — initial, final, negotiated saving,
  variance vs budget, saving vs highest, "lowest" chip), a **rate-comparison** grouped column
  chart (`components/RateComparisonChart.tsx`, inline SVG, validated blue/orange categorical
  pair, light+dark), then a collapsible **per-vendor** section with the initial/final item table
  (rows paired server-side on the description) and both totals blocks. Cross-vendor metrics skip
  quotes in another currency (`mixed_currency` says so) — converting would invent a rate. The
  sheet's **UoM** column has no data behind it and the **recommendation block is out of scope**
  (that decision comes after the comparison and has its own card). Endpoints `GET/POST/PUT/DELETE
  /purchase-requests/{id}/quotation-comparison` (read = `callerCanView`; writes = procurement +
  team-lead 409 + `assignmentWorkGate`); process events `create|update|delete_quotation_comparison`.
  All **detail pages were widened 800/900 → 1200** (the width the list pages already use) to give
  the card room.
- **User directory (SCIM autocomplete)** — see `docs/user-directory.md` (issue #2496). Name/email
  form fields offer type-to-search suggestions sourced from the connected identity server (WSO2 IS /
  Asgardeo) via its **SCIM2 Users** API. The backend (`internal/directory`) fetches the **full**
  directory once per TTL into an in-memory cache (single-flight refresh; ~1000 users, so cache-all +
  client-side filter beats live-per-keystroke) using an OAuth2 **client-credentials** M2M token
  (scope `internal_user_mgt_list`); the browser never sees SCIM or the credentials. Config is a new
  `scim` block in `config.yaml` — **when `scim.enabled` is false (default), it falls back to the
  app's active DB users**, so dev needs no M2M app. `GET /api/v1/users/directory` (any user) returns
  the cached `[{name,email}]`; `POST /api/v1/users/ensure` (procurement_admin/admin) get-or-creates a
  DB user by email (`GetOrCreateUserByEmail`, reusing the invite-by-email path) so **id-based**
  pickers can select someone who hasn't logged in. Frontend: `EmailAutocomplete` (free-text +
  suggestions — accepts a typed email not in the directory) on **team-lead email** (create + edit),
  **budget-chain steps** (add/edit), and the **invite-user** form; `DirectoryUserPicker` (search →
  ensure → id) on **business-unit approvers** and **team members**. PR assignee/collaborators stay
  DB/procurement-scoped (the backend requires the procurement role there). Both pickers filter the
  cached directory through **`lib/directorySearch.ts` (`searchPeople`)**, which *ranks* rather than
  merely filters: full name/email **prefix** first, then a prefix of any name word or email
  local-part segment (surnames, `first.last@`), then all query tokens matching word prefixes in any
  order, and only last a plain substring hit — so incidental mid-word matches no longer crowd out the
  person being typed. Ties break on display name, then email; `foundCount` (uncapped) still drives
  `useRefreshOnNoMatch`. No migration, no new event action (reads + reuses the invite mechanism).
