# Refactor the Purchase Requisition form to the ProQ design

## Context

The new-requisition wizard (`webapp/src/components/RequisitionForm.tsx`, shared by create + edit)
is being reworked to match the `resources/ProQ-new-requisition.html` mockup — both its visual
language ("ProQ" dark hero, Space Grotesk / IBM Plex Mono / Inter, pill stepper, section bands, type
cards) and its structure (5 steps, 3 categories, richer IT fields). The mockup deliberately drops
several fields the current form collects; per the decisions below we follow it, keeping the backend
columns intact (non-destructive) but no longer surfacing them.

### Decisions taken (from clarifying questions)
- **Follow the mockup literally** for fields: drop Budget unit, WSO2 entity, and the config-list
  budget-coding selects from the form (replace coding with free text). *Keep the DB columns* and the
  `PurchaseRequestInput`/`PRDetails` type fields so editing legacy PRs stays non-destructive — the
  form simply stops surfacing/setting them (new PRs get `budget_unit_id=null`, `entity=''`).
- **Estimated value + Currency: dropped** from the form (default to `0` / `USD`).
- **Team-lead email: kept** (backend requires it; mandatory approval gate). Placed in step 1.
- **Marketing & Events: real category** — `category` is unconstrained free text (no CHECK, no Go
  validation, `transitions.go` doesn't switch on it), so the backend already accepts `"EVENTS"`. Work
  is frontend-only: add it to `PRCategory`, render its "under development" panels, handle it in the
  read-only view + title. Requirement/vendor questions are placeholders per the mockup.
- **Style the form only** — restyle the requisition form + its two host pages; leave `Layout.tsx`
  (app nav/header) untouched.

### Consequence to handle: budget-approver resolution
With `budget_unit_id=null`, `resolve_budget_approvers` returns nobody, so a recommendation's **base
budget step / budget card becomes admin-only** (`repository/recommendations.go` `IsBudgetApproverForPR`;
list predicates in `repository.go`). The form still collects the free-text `budget_approver_email`
(required). **Recommended (included below): a minimal backend mitigation** so the base budget step
falls back to matching `purchase_requests.budget_approver_email` when `budget_unit_id` is null — this
keeps budget approval working for non-admins. This is the only backend change and can be deferred if
you'd rather accept the admin-only behaviour.

## Approach

### 1. Fonts + ProQ styles (`webapp/index.html`, new `webapp/src/components/RequisitionForm.css`)
- Add the Google Fonts `<link>` for `Space Grotesk`, `IBM Plex Mono`, `Inter` to `index.html`.
- Port the mockup's CSS into a scoped stylesheet `RequisitionForm.css`, prefixing every selector under
  a wrapper class (e.g. `.proq-req`) so styles don't leak into the rest of the Tailwind app. Import it
  from the component. This is the fastest faithful path for this ornate, self-contained design (the app
  otherwise uses plain Tailwind; matching this visual by hand in utility classes would be error-prone).
- The dark **hero banner + pill stepper** live at the top of the form card region; rendered inside the
  existing light `Layout` header (acceptable — banner is a distinct dark block).

### 2. Types (`webapp/src/types/api.ts`)
- `PRCategory = "IT" | "NON-IT" | "EVENTS" | ""`.
- Extend `PRDetails` with the new JSONB fields (no migration — dev-app JSONB blob):
  `business_unit?`, `it_user_count?`, `it_user_names?: string[]`, `it_admin_count?`,
  `it_admin_names?: string[]`, `sec_employee_pii_detail?`, `sec_integration_systems?`,
  `sec_integration_kind?`, `sec_vendor_docs?: YesNo`, `sec_vendor_docs_link?`, `supplier_phone?`.
  Reuse existing fields where they map: `it_usage` stores the usage-period select value;
  `budget_category`/`budget_product`/`budget_region`/`engagement_code` stay but become free-text inputs.
- Leave `budget_unit_id`, `entity`, `estimated_value`, `currency` in `PurchaseRequestInput` (passed
  through on edit; form no longer renders them).

### 3. Rewrite `RequisitionForm.tsx` — 5 steps (keep exported `emptyRequisition` + `requisitionTitle` signatures)
- **Step 1 — Requester details**: Date, full name, WSO2 email (3-col) + SSO help note; **Team lead
  email** (required, kept). Drop budget unit + entity from here.
- **Step 2 — Procurement Category**: 3 selectable type cards (IT / Non-IT / Marketing & Events).
- **Step 3 — Requirement Details** (dynamic per category):
  - *IT*: product name, plan/tier, description; user count + dynamic **name list**, admin count +
    dynamic name list (counts pre-seed rows, like the mockup's `syncNames`); usage-period select;
    business justification; **Data & security assessment** (sensitive; external PII + follow-up;
    employee PII + follow-up; integrates → systems + kind select + vendor-docs yes/no + link).
  - *Non-IT*: goods/services details, additional specs/links, business justification. (Drop the
    `nit_category` select.)
  - *Events*: "under development" placeholder panel.
- **Step 4 — Vendor & budget** (dynamic per category):
  - *IT/Non-IT*: proposed supplier (name, website, contact name, contact email; Non-IT adds phone) +
    **attach documents** file input; info-note; **Budget details** as free text (budget category,
    business unit, product, region, engagement code); **Budget approval** (approver name* + email*,
    both required); notes for procurement. (No estimated value / currency / budget-unit picker /
    designated-approver reference.)
  - *Events*: "under development" placeholder.
- **Step 5 — Review & submit**: 4 review blocks with "Edit" buttons that jump back; per-category rows;
  `requireDeclaration` checkbox (create only) preserved.
- Reuse `SupplierNameCombobox` for supplier name (keeps vendor-master auto-fill + `supplier_vendor_id`).
  Drop `useConfigLookup`/`optionsFor`, `useBudgetUnitLookup`/`useBudgetUnitApprovers`,
  `BudgetApproverReference`, `Collapsible`, `CurrencyInput` from this component (still used elsewhere).
- `requisitionTitle`: add `EVENTS` branch (generic "Marketing & Events requisition"); IT keeps
  `it_product` fallback, Non-IT keeps a category/description fallback.
- `emptyRequisition`: unchanged shape; `budget_unit_id:null`, `entity:""`, `estimated_value:0`,
  `currency:"USD"`, `category:""`.

### 4. Attachments (create flow)
- Form stages `File[]` for the supplier attach field via new optional props
  (`attachments`, `onAttachmentsChange`); rendered only when a category is chosen.
- `NewPurchaseRequestPage.tsx`: after `createPurchaseRequest` succeeds, upload each staged file with
  the existing `uploadDocument(pr.id, file)` (`api/purchaseRequests.ts:96`) before navigating.
- In **edit** mode the attach field is hidden (the detail page's existing Documents card manages docs).

### 5. Host pages
- `NewPurchaseRequestPage.tsx` / `PurchaseRequestDetailPage.tsx`: the breadcrumb/`h1` chrome moves into
  the ProQ hero (or is removed where the hero replaces it); `RequisitionForm` call sites keep the same
  props (add `attachments` on create). `toInput` unchanged (still carries legacy columns through).
- `PurchaseRequestDetailPage` **ReadOnlyView** (~lines 555-600): update the Purchase/Vendor sections to
  render the new `PRDetails` fields and add an `EVENTS` "under development" branch + Type label.

### 6. Backend mitigation (recommended; `backend/internal/repository/recommendations.go` + `repository.go`)
- In `IsBudgetApproverForPR` and the base-step actionable path, add a fallback: when
  `pr.budget_unit_id IS NULL`, qualify the caller if their email is a (case-insensitive) member of
  `purchase_requests.budget_approver_email` (comma-separated). Mirror in the list/visibility predicates
  (`approvablePredicate`, `myApprovalStateExpr`) and `BudgetApproversForPR` (notifications).
- No migration; no data-model change.

## Files to modify
- `webapp/index.html` — font links.
- `webapp/src/components/RequisitionForm.tsx` — full rewrite (5-step ProQ form).
- `webapp/src/components/RequisitionForm.css` — **new**, scoped ProQ styles.
- `webapp/src/types/api.ts` — `PRCategory` + `PRDetails` additions.
- `webapp/src/pages/NewPurchaseRequestPage.tsx` — hero chrome + post-create attachment upload.
- `webapp/src/pages/PurchaseRequestDetailPage.tsx` — ReadOnlyView field/EVENTS updates.
- `backend/internal/repository/recommendations.go`, `repository.go` — budget-approver fallback (mitigation).
- Docs (repo convention): archive this plan to `docs/plans/NN-proq-requisition-form.md`; add/adjust the
  CLAUDE.md Phase-1 bullet to note the ProQ 5-step form, the EVENTS category, and the dropped fields.

## Verification
- `cd webapp && npm run build` (tsc + vite) — no type errors from the `PRCategory`/`PRDetails` changes.
- `cd backend && go build ./... && go vet ./...`; run existing repo tests
  (`go test ./internal/repository/...`) for the approver-fallback change.
- Run the app (`/run` skill or manual: backend `go run ./cmd/server`, `webapp npm run dev`):
  1. **Create** a PR in each category — IT (fill name lists, security follow-ups, integration), Non-IT,
     Events (placeholder) — step through all 5 steps, confirm required-field guards, submit, land on
     the detail page. Attach a file on create and confirm it appears in the Documents card.
  2. **Edit** an existing (legacy) PR — confirm its category/details load, legacy `budget_unit_id`/
     `entity` are preserved on save (non-destructive), and Save works.
  3. Confirm the ReadOnlyView renders the new fields + EVENTS branch.
  4. Budget mitigation: on a PR with a free-text budget approver, create a recommendation and confirm
     that approver (non-admin) can act on the budget card.
- Clean up any test PRs created (per CLAUDE.md "Data clean up").
