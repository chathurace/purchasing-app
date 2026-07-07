# Plan: Configurable dropdowns + Settings page, and cost-center / business-unit cleanup

## Context

Every dropdown in the requisition wizard (WSO2 entity, IT/Non-IT category, currency,
engagement type, budget category, product, region) is a **hardcoded array** in
`frontend/src/types/api.ts` (lines 228–284). Changing any option needs a code change + redeploy.
The user wants all of these editable at runtime from a new **Settings page**, and the
**engagement code** — today a free-text input — turned into a configurable dropdown too.

Separately, the app has a confusing overlap between "cost center", "business unit", and "team".
Per the user's decisions:

1. **Keep the "Cost Center" entity name.** Do **not** rename it to business unit.
2. **Remove the `business_unit` association from cost centers** (the tag column added in migration 023).
3. **Rename all "business unit" references in the forms to "cost center."**
4. **Settings page access** = admin **and** finance_admin (matches existing `HasCostCenterAdmin`).
5. **Budget-owner approval** = the form picks a **real Cost Center** (from the managed list); the
   budget approval card actor = that cost center's primary/secondary owner. This replaces today's
   `cost_centers.business_unit == purchase_requests.team` matching.

**Resolution of the field overlap** (flagged to the user): rather than ending up with two fields
both labelled "Cost center", the single top-of-form **"Team / Business unit"** select becomes the
**real Cost Center picker** (binds `cost_center_id`), and the now-redundant budget-coding
**"Business unit (BU)"** field (`budget_bu`) is **removed**.

---

## Part A — Configurable dropdowns + Settings page

### A1. Backend: generic option-set store

A single generic table backs every configurable list (keyed by `list_key`), rather than one
table per dropdown.

**Migration `024_config_options.sql`**
```sql
CREATE TABLE config_options (
    id         BIGSERIAL PRIMARY KEY,
    list_key   TEXT    NOT NULL,
    value      TEXT    NOT NULL,
    sort_order INT     NOT NULL DEFAULT 0,
    is_active  BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (list_key, value)
);
CREATE INDEX idx_config_options_key ON config_options(list_key, sort_order);
```
Seed it with the current constant values (preserving order via `sort_order`) for keys:
`entity`, `it_category`, `nonit_category`, `engagement_type`, `currency`, `budget_category`,
`product`, `region`. Seed `engagement_code` with a small starter set (or leave empty — it was
free-text before). The `team` and `business_unit` lists are intentionally **not** created (replaced
by the cost-center picker / removed).

**Known-list registry** — add to `internal/model/model.go` a slice of `{Key, Label}` describing the
configurable lists (drives both server-side `list_key` validation on create and the Settings-page
section order/titles). Returned via the lookup endpoint so the frontend isn't hardcoded either.

**Repository** `internal/repository/config_options.go` (mirror the cost-center repo style):
- `ListConfigOptions(ctx)` — all rows incl. inactive, ordered by `list_key, sort_order` (settings page).
- `ConfigOptionsLookup(ctx)` — active rows only, grouped into `map[string][]string` (forms).
- `CreateConfigOption`, `UpdateConfigOption` (value/sort_order/is_active), `DeleteConfigOption`.

**Handler** `internal/handler/config_options.go` (mirror `cost_centers.go`):
- `GET  /config/options/lookup` — any authenticated user; returns `{ lists: {...}, keys: [{key,label}] }`.
- `GET  /config/options` — `HasCostCenterAdmin` gate; full list incl. inactive.
- `POST /config/options` — create `{list_key, value, sort_order}`; reject unknown `list_key`.
- `PUT  /config/options/{id}` — update `{value, sort_order, is_active}`.
- `DELETE /config/options/{id}` — delete.

Register in `internal/handler/router.go` (lookup before `/{id}`, same as cost-centers at lines 100–106).
Gate mutations with `HasCostCenterAdmin` (already = admin + finance_admin); optionally add a
`HasSettingsAdmin` alias in `internal/middleware/auth.go` for readability.

### A2. Frontend: consume + manage

- `src/api/config.ts` + `src/hooks/useConfigOptions.ts` — TanStack Query hook over
  `/config/options/lookup`, returning `Record<string,string[]>` plus the key/label registry.
  Provide `useOptionList(key)` returning the active values with the hardcoded arrays kept **only as
  fallback** while loading/empty.
- **Rewire `RequisitionForm.tsx`**: replace `IT_CATEGORIES`, `NONIT_CATEGORIES`, `ENGAGEMENT_TYPES`,
  `REQ_CURRENCIES`, `BUDGET_CATEGORIES`, `PRODUCTS`, `REGIONS`, and `WSO2_ENTITIES` `Select` `options`
  props with `useOptionList(...)`. Make **engagement code** a `Select` over `useOptionList("engagement_code")`
  (was an `<input>` at line 487–489). **Important:** when editing an existing PR whose stored value
  is no longer in the active list, inject that value as an extra option so it still displays/saves.
- **Settings page** `src/pages/SettingsPage.tsx` at route `/settings` (register in `App.tsx`; add a
  nav link in `components/Layout.tsx`, shown only to admin/finance_admin like the Users link).
  One collapsible section per registry key; each lists its values with add / rename / reorder
  (sort_order) / activate-deactivate / delete, following `UserManagementPage.tsx` mutation patterns.
- Remove the now-unused `TEAMS` / `BUSINESS_UNITS` / `*_CATEGORIES` etc. constant exports from
  `types/api.ts` once nothing imports them (keep any still referenced as fallbacks, renamed `DEFAULT_*`).

---

## Part B — Cost-center / business-unit cleanup

### B1. Drop `business_unit` from cost centers
- **Migration `025_drop_cost_center_business_unit.sql`**: `DROP INDEX idx_cost_centers_bu;`
  `ALTER TABLE cost_centers DROP COLUMN business_unit;`
- Backend: remove `BusinessUnit` from `CostCenter`/`CostCenterInput` structs, `costCenterCols`,
  `scanCostCenter`, the INSERT/UPDATE in `repository/procurement.go`, and `business_unit` from
  `costCenterInput`/`toRepo()` in `handler/cost_centers.go`.
- Frontend: remove the "Business unit" select from `components/CostCenterFields.tsx` (lines 74–86)
  and the `business_unit` field from the cost-center types/initial-state in `types/api.ts`
  (lines 373, 388, 683) and `pages/CostCenterDetailPage.tsx` (line 28).

### B2. Top-of-form field → real Cost Center picker
- In `RequisitionForm.tsx` step 1 (lines 301–303): replace the `TEAMS` `Select` with a
  cost-center picker sourced from `/cost-centers/lookup` (reuse the existing `useCostCenterLookup`
  hook + the `VendorSelect`/`CostCenterFields` select pattern). Bind to `value.cost_center_id`
  (and store the chosen name in `value.cost_center` for display/history). Label = **"Cost center"**,
  hint = "Determines who owns the budget approval." Update the step-1 validation
  (line 64) and the Review summary (line 516) accordingly.
- The PR create/update handler already accepts `cost_center` + `cost_center_id`
  (`handler/purchase_requests.go:51–52, 71–72`), so no API shape change is needed — just send them
  from the form. The `team` column becomes unused by the new form (left at its `''` default;
  retained for legacy rows).

### B3. Remove the redundant budget-coding BU field
- Remove the "Business unit (BU)" `Field` + `budget_bu` `Select` (`RequisitionForm.tsx` 476–478),
  its validation entry (line 96), and Review row (line 525). Drop `budget_bu` from `PRDetails`
  (`types/api.ts:221`) and the display row in `PurchaseRequestDetailPage.tsx:548`.
- Relabel the detail-page "Team / Business unit" row (`PurchaseRequestDetailPage.tsx:494`) to show
  the **cost center** name instead of `pr.team`.

### B4. Rewire budget-owner derivation to `cost_center_id`
- `repository/recommendations.go` `IsBudgetOwnerForPR` (lines 454–466): change the join from
  `JOIN cost_centers cc ON cc.business_unit = pr.team AND pr.team <> ''` to
  `JOIN cost_centers cc ON cc.id = pr.cost_center_id`.
- `repository/procurement.go` `ListPurchaseRequests` budget-visibility subquery (~lines 523–528):
  change the `cc.business_unit = pr.team` match to `cc.id = pr.cost_center_id` so cost-center owners
  still see PRs awaiting their budget card.
- No change needed in `handler/purchase_requests.go canActOnRecType` — it already delegates to
  `IsBudgetOwnerForPR`.

---

## Files to create / modify

**Backend (new):** `migrations/024_config_options.sql`, `migrations/025_drop_cost_center_business_unit.sql`,
`internal/repository/config_options.go`, `internal/handler/config_options.go`.
**Backend (edit):** `internal/model/model.go` (list registry), `internal/handler/router.go`,
`internal/middleware/auth.go` (optional alias), `internal/handler/cost_centers.go`,
`internal/repository/procurement.go`, `internal/repository/recommendations.go`.

**Frontend (new):** `src/api/config.ts`, `src/hooks/useConfigOptions.ts`, `src/pages/SettingsPage.tsx`.
**Frontend (edit):** `src/types/api.ts`, `src/components/RequisitionForm.tsx`,
`src/components/CostCenterFields.tsx`, `src/pages/PurchaseRequestDetailPage.tsx`,
`src/pages/CostCenterDetailPage.tsx`, `src/App.tsx`, `src/components/Layout.tsx`.

**Docs (per CLAUDE.md convention):** archive this plan to `docs/plans/06-configurable-dropdowns.md`
and add a companion `docs/settings.md` as-built reference after the phase.

---

## Verification

1. **Migrations**: `psql -h localhost -d purchasing -f backend/migrations/024_config_options.sql`
   then `...025_...sql`. Confirm `config_options` is seeded and `cost_centers.business_unit` is gone.
2. **Backend**: `cd backend && go build ./... && go run ./cmd/server`. Smoke-test
   `GET /api/v1/config/options/lookup` (any user) and the CRUD endpoints (as admin/finance_admin →
   200; as plain staff → 403 on mutations).
3. **Frontend**: `cd frontend && npm run dev`.
   - As admin/finance_admin, open **/settings**: add/rename/reorder/deactivate a value in e.g.
     "WSO2 entity" and confirm it appears (and the deactivated one disappears) in the new-request form.
   - Create a PR: the step-1 **Cost center** dropdown lists managed cost centers; engagement code is
     now a dropdown; no "Business unit" fields remain anywhere.
   - As the owner of the selected cost center, confirm the PR appears in their list and they can
     approve the **budget** recommendation card; a non-owner cannot.
   - Edit an older PR and confirm a stored value not in the active list still displays.
4. `cd backend && go vet ./...` and run any existing tests.

## Caveats
- PRs created by the *previous* requisition form have `cost_center_id = NULL`; their budget approval
  card will have no derived actor until a cost center is assigned (admin still passes). This affects
  only in-flight PRs in dev and is acceptable.
- Currency is made configurable for the requisition form; other currency dropdowns
  (cost center / quotation / invoice) can optionally be pointed at the same `currency` list in a
  follow-up — out of scope unless you want it now.
