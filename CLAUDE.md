# Purchasing App — contributor onboarding

Internal app to manage the company's purchasing function. Greenfield sibling of `../finance-apps`
(same stack), but simpler: a CRUD app for purchase requests with role-based views and local file
storage. No workflow engine.

## Stack

- **Backend** (`backend/`): Go 1.25 · `go-chi/chi/v5` · `jackc/pgx/v5` (pgxpool) ·
  `coreos/go-oidc/v3` (SSO) · `rs/zerolog`. Config from **`config.yaml`** (not env vars).
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
  procurement users associate a quotation inline on the PR page (vendor + optional description + a PDF)
  without leaving it. The quotation detail page still carries the richer fields (line items, total,
  currency, valid-until). A quotation has a **single primary "quotation PDF"** plus **zero or more
  other documents** (migration `036`, `quotations.quotation_document_id` FK → `documents`, mirroring
  `contracts.signed_document_id`): the primary is pulled out of the doc list on detail reads
  (`quotation_document` vs `documents`), managed at `.../quotations/{id}/quotation-document`
  (upload replaces; delete removes) while other docs use `.../quotations/{id}/documents`. On the PR
  page each quotation is an **expandable row** (basic info + a link to download the quotation PDF and
  any other docs; lazily fetched via `GetQuotation`). Selecting a quotation advances the PR to `vendor_selected`; the contract
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
