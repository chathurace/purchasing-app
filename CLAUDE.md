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

`staff`, `procurement`, `procurement_admin`, `admin`, plus `legal` / `security` (the legal/security approval
cards on a PR's procurement recommendation; they also retain read access to contracts). New SSO
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
  request, view/edit request. The new-request form is the **WSO2 SOP-85000 requisition wizard**
  (`components/RequisitionForm.tsx`, shared by create + edit): a 4-step flow (Requester / Purchase
  [IT vs Non-IT toggle + IT data-security assessment] / Vendor & budget / Review). Core fields are
  columns on `purchase_requests` (`team`, `entity`, `category`, `estimated_value`, `currency`,
  `budget_approver_name/email`, derived `title`); the rest of the form is a `details` **JSONB** blob
  (migration `023`, shape = `PRDetails` on the client). Requesters no longer pick system approvers —
  procurement sign-off is the recommendation (below). Old `pr_items`/`pr_links` are unused by the
  new form (kept for legacy rows).
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
  required approvals: budget owner / legal / security, budget default-checked) inline on the PR page.
  Each required approval is an **approval card** with an approve toggle and a comment thread (text +
  doc attachments, owner type `pr_recommendation_comment`). Card actors: legal→`legal` role,
  security→`security` role, budget→a **qualified budget approver** of the PR's **budget unit** (see
  the **Budget units** entry below): the approver(s) of the bracket that the *recommendation's*
  estimated value + currency resolve to (`IsBudgetApproverForPR` via `resolve_budget_approvers`, falling
  back to the unit's default approver); any of them may approve, and all are notified (admin passes any).
  The card a caller may act on is server-computed
  into `recommendation.my_actionable_types` (comment/view) plus per-card `can_comment`/`can_approve`/
  `can_assign` flags — see the **Teams & approval assignees** entry below for the legal/security
  assignee split (comment = whole team, approve = the assignee only).
  **All required cards must be approved before a quotation can be selected**
  (`RecommendationFullyApproved`, enforced in `SelectQuotation`). Editing the recommendation resets all cards to pending. PR view/list
  access is extended so card actors can see and find the PRs awaiting them. The recommendation view
  renders each item as an elevated **sub-card** (`components/RecommendationSection.tsx` →
  `SubCard`/`IconTile`): the approval cards, plus two optional cards procurement manages inline:
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
  master-data/admin mutations (vendors, budget units, config options, users/roles, storage). The
  `action` set is fixed in Go (`model.ValidProcessActions`/`ValidAuditActions`, in
  `internal/model/events.go`) and validated on write — a task-decision direction (approve/reject,
  sign/unsign, target status) is a `qualifier`, not a new action. Recorded best-effort at the handler
  layer (`recordProcessEvent`/`recordAuditEvent`) after a successful mutation; PR status
  auto-advances are *not* recorded (derivable). v1 is write-only (no API/UI); query the tables
  directly.
- **Teams & approval assignees** — see `docs/teams-approval-assignee.md` (migrations `038` teams,
  `039` assignee). A **team** (`teams` table, seeded fixed set Legal/Security/Procurement) is a name
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
  legal/security recommendation approval cards gain an **assignee**
  (`pr_recommendation_approvals.assignee_id`): the previously all-or-nothing card is split three ways,
  serialized as `can_comment`/`can_approve`/`can_assign` — **comment** = any team member (as before),
  **approve** = **the assignee only** (`canApproveRecType`), **assign** = any team member or procurement
  (`canAssignRecType`); budget card unchanged. Setting the assignee prompts to notify (email + link,
  CC the `team_email`); a **Send reminder** button re-sends. Email uses the existing SMTP mailer
  (`Mailer.SendCC` added for the CC); disabled in dev = logged. Endpoints
  `PUT/POST .../recommendation/approvals/{type}/assignee[/remind]`; process events
  `assign_rec_legal`/`assign_rec_security` (qualifier `assign`/`unassign`).
- **Budget units (was cost centers)** — see `docs/budget-units.md` (migrations `041`/`042`; dev-only,
  no data migration — `041` **truncates** existing cost-center data). `cost_centers` →
  **`budget_units`**; the old primary/secondary-owner model is dropped and replaced by value-based
  **approval brackets** plus a **default approver** (`budget_units.default_approver_id`, required). A
  budget unit has one or more ordered `budget_unit_brackets` (own `currency`, `min_value`, `max_value`
  NULL = unbounded) each with one or more `budget_unit_bracket_approvers`. The **budget approver** is
  derived, not stored: SQL function `resolve_budget_approvers(bu_id, value, currency)` returns the
  approvers of the first bracket (by position) whose **own currency** matches and whose range contains
  the value; otherwise (no value / no currency-matched bracket / out of range) it returns the unit's
  **default approver**. All returned approvers qualify (any may approve; all are notified — there is no
  "first"). Two resolution
  points: the **requester's** `estimated_value` drives a creation-phase **preview**
  (`GET /budget-units/{id}/approvers?value=&currency=`, shown on the requisition form and PR page); the
  **recommendation's** `estimated_value` + `currency` drive the **actual** budget-card approver
  (`IsBudgetApproverForPR` + the two list/dashboard predicates in `repository.go`, all via the
  function). `purchase_requests.cost_center_id` → `budget_unit_id` (legacy free-text `cost_center`
  dropped); `invoice_cost_allocations.cost_center_id` → `budget_unit_id`. Management (create/edit
  brackets, deactivate) is procurement_admin/admin at `/budget-units` (`middleware.HasBudgetUnitAdmin`,
  which also gates config options); the active-unit lookup is open. Procurement can re-notify budget
  approvers via `POST .../recommendation/approvals/budget/remind`; approvers are also emailed on
  recommendation create/update. Audit actions `create_budget_unit`/`update_budget_unit`, entity
  `budget_unit`. The resolved approver(s) are called the **designated approver(s)** in the UI. The
  requisition form keeps an **editable** budget approver (`purchase_requests.budget_approver_name/email`)
  with a "use designated" shortcut; a mismatch with the designated approver is flagged on the Review
  step (non-blocking). The recommendation budget card shows that named approver — procurement edits it
  inline or syncs it to the designated one via `PUT /purchase-requests/{id}/budget-approver`
  (`SetBudgetApprover`, procurement; touches only the approver fields).
