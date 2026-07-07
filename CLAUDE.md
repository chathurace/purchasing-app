# Purchasing App — contributor onboarding

Internal app to manage the company's purchasing function. Greenfield sibling of `../finance-apps`
(same stack), but simpler: a CRUD app for purchase requests with role-based views and local file
storage. No workflow engine.

## Stack

- **Backend** (`backend/`): Go 1.25 · `go-chi/chi/v5` · `jackc/pgx/v5` (pgxpool) ·
  `coreos/go-oidc/v3` (SSO) · `rs/zerolog`. Config from **`config.yaml`** (not env vars).
- **Frontend** (`frontend/`): React 18 + TypeScript · Vite · React Router v6 · TanStack Query v5 ·
  `oidc-client-ts` · Tailwind CSS.
- **DB**: local PostgreSQL, database `purchasing`.
- **Files**: stored on local disk under `storage.files_root` (default `purchasing-app/files`),
  original filenames preserved.

## Roles (maintained in-app, not from the SSO token)

`staff`, `finance`, `finance_admin`, `admin`, plus `legal` / `security` (the legal/security approval
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
cd frontend && npm install && npm run dev   # http://localhost:5173
```

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
- **Phase 2 (finance view)** — see `docs/plans/02-finance-view.md` (plan archive) and
  `docs/contracts.md` (as-built contract model). The procurement workflow up to contract signing:
  vendors, quotations (with line items), and contracts. **RFQs were removed** (migration `021`): a PR
  now has zero or more quotations attached **directly** (`quotations.purchase_request_id`), and
  finance users associate a quotation inline on the PR page (vendor + optional description + a PDF)
  without leaving it. The quotation detail page still carries the richer fields (line items, total,
  currency, valid-until). Selecting a quotation advances the PR to `vendor_selected`; the contract
  itself is created and managed from the recommendation's **contract card** (below). **Contract
  review/approval was removed** (migration `032`): a contract is simply `draft` until its **signed
  PDF** is attached, then `signed` — approvals live only on the PR (the recommendation cards). A
  contract now holds one or more **draft-contract PDFs** plus a single **signed PDF**, each with its
  own `notes` (`documents.notes`, migration `031`); see `docs/contracts.md`. Any
  `finance`/`finance_admin`/`admin` user performs all procurement actions (helper
  `middleware.HasFinanceAccess`); `staff` is unchanged. Statuses auto-advance via
  `model/transitions.go` (the first quotation moves a PR `submitted → under_review`; PR terminal
  state for this phase is `order_signed`; `completed` is reserved for the future payment phase). The
  `documents` table is generalized with `owner_type`/`owner_id` so quotations/contracts share it
  while files still live under the owning PR's storage dir. NetSuite PO / payment settlement remain
  out of scope.
- **Phase 3 (contract fulfillment)** — see `docs/plans/03-invoices-grns.md`. Once a contract is
  `signed`, finance users record one or more **GRNs** (goods received notes) and **invoices**
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
  `repository/recommendations.go` + `handler/recommendations.go`. Once a PR has ≥1 quotation, finance
  adds a **single** recommendation (vendor — restricted to a quoted vendor — + description + the
  required approvals: budget owner / legal / security, budget default-checked) inline on the PR page.
  Each required approval is an **approval card** with an approve toggle and a comment thread (text +
  doc attachments, owner type `pr_recommendation_comment`). Card actors: legal→`legal` role,
  security→`security` role, budget owner→the owner of the cost center whose `business_unit` (migration
  `023`) matches the PR's `team` (admin passes any) — i.e. budget ownership is derived from the
  requester's team, not a per-PR cost-center pick. The card a caller may act on is server-computed
  into `recommendation.my_actionable_types`.
  **All required cards must be approved before a quotation can be selected / a contract drafted from a
  quotation** (`RecommendationFullyApproved`, enforced in `SelectQuotation` +
  `CreateContractFromQuotation`). Editing the recommendation resets all cards to pending. PR view/list
  access is extended so card actors can see and find the PRs awaiting them. The recommendation view
  renders each item as an elevated **sub-card** (`components/RecommendationSection.tsx` →
  `SubCard`/`IconTile`): the approval cards, plus two optional cards finance manages inline:
  - **RFI card** (migration `030`; `rfi_description` on `pr_recommendations` + docs owner type
    `pr_recommendation_rfi`): a request-for-information raised with the vendor — a description plus one
    or more PDF attachments. Endpoints under `.../recommendation/rfi`.
  - **Contract card** (migration `029`; `pr_recommendations.contract_id`): creates/links a real
    contract (so it appears on the Contracts page) via `CreateRecommendationContract` — **ungated**
    (unlike the quotation path). It embeds the shared `ContractContent` (draft-contract PDFs + signed
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
- **User management (admin)** — see `docs/user-management.md`. Admins add users *by email* (pending
  invites — `users.sub` is now nullable, claimed on first login by email match), grant/revoke roles
  (`staff` is a non-removable baseline), and deactivate users (`users.is_active`; blocks login). A
  last-admin guard prevents lockout. UI at `/users` (admin-only nav link); API under `/api/v1/users`.
