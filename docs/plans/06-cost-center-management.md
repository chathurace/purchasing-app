# Cost Center Management

## Context

Cost centers currently exist only as a **free-text string** on purchase requests
(`purchase_requests.cost_center`, rendered as a plain text input in
[RequestFields.tsx](purchasing-app/frontend/src/components/RequestFields.tsx#L37-L44)). There is no
master list, so values are inconsistent and unmanaged.

We will introduce a **Cost Center master entity** with a dedicated management page (admin /
finance_admin only) supporting create / edit / deactivate, and **link purchase requests** to it via
a dropdown that replaces the free-text field.

This mirrors the existing **Vendor** feature almost exactly — it is the blueprint for an
admin-gated CRUD master — and reuses the **PR approvers** many-to-many pattern for secondary owners.

### Decisions (confirmed with user)
- **Fields:** `name` (required), `code`, `description`, primary owner, secondary owners, `budget`,
  `currency`, `is_active`. Only **name is required**.
- **Deletion:** soft delete via `is_active` toggle (deactivate / reactivate), like vendors. No hard delete.
- **PR linkage:** convert the PR cost-center field into a dropdown selecting from active cost centers.
- **Access:** management page + write APIs limited to `admin` / `finance_admin`. A lightweight
  lookup endpoint is readable by any authenticated user (so staff can pick one on a PR).

---

## Backend (Go)

### 1. Migration — `backend/migrations/018_cost_centers.sql` (new)
Model on [006_vendors.sql](purchasing-app/backend/migrations/006_vendors.sql) and
[017_pr_approvals.sql](purchasing-app/backend/migrations/017_pr_approvals.sql).

```sql
CREATE TABLE cost_centers (
    id               BIGSERIAL     PRIMARY KEY,           -- displayed as CC-{id:06d}
    code             TEXT          NOT NULL DEFAULT '',
    name             TEXT          NOT NULL,
    description      TEXT          NOT NULL DEFAULT '',
    primary_owner_id BIGINT        REFERENCES users(id),
    budget           NUMERIC(16,2) NOT NULL DEFAULT 0,
    currency         TEXT          NOT NULL DEFAULT '',
    is_active        BOOLEAN       NOT NULL DEFAULT TRUE,
    created_by       BIGINT        REFERENCES users(id),
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_cost_centers_name ON cost_centers(name);
-- code optional, but unique when present
CREATE UNIQUE INDEX idx_cost_centers_code ON cost_centers(code) WHERE code <> '';

-- secondary owners: many-to-many (mirror of pr_approvals shape)
CREATE TABLE cost_center_secondary_owners (
    cost_center_id BIGINT NOT NULL REFERENCES cost_centers(id) ON DELETE CASCADE,
    user_id        BIGINT NOT NULL REFERENCES users(id),
    PRIMARY KEY (cost_center_id, user_id)
);

-- link purchase requests to the master (nullable; legacy text column kept for history)
ALTER TABLE purchase_requests ADD COLUMN cost_center_id BIGINT REFERENCES cost_centers(id);
```
Apply with `psql -h localhost -d purchasing -f backend/migrations/018_cost_centers.sql`.

### 2. Repository — `backend/internal/repository/procurement.go`
Add structs + methods next to the Vendor ones (`CreateVendor`/`ListVendors`/… are the template):
- `CostCenter` (read model, includes `PrimaryOwner *UserSummary` and `SecondaryOwners []*UserSummary`)
  and `CostCenterInput` (write model: code, name, description, primaryOwnerID `*int64`, budget,
  currency, isActive, secondaryOwnerIDs `[]int64`).
- `CostCenterSummary` (`id`, `code`, `name`) for the lookup endpoint.
- `ListCostCenters(ctx)` — all, ordered by name; join primary owner; aggregate secondary owners.
- `ListActiveCostCenters(ctx)` — `WHERE is_active`, summary only (for PR dropdown).
- `GetCostCenter(ctx, id)`, `CreateCostCenter(ctx, in, createdBy)`, `UpdateCostCenter(ctx, id, in)`.
  Create/Update run in a **transaction**: upsert the row, then replace secondary-owner rows
  (delete-all + re-insert, the same insert-loop idiom used for approvers in
  [repository.go:385](purchasing-app/backend/internal/repository/repository.go#L385)).
- `GetCostCenterUsage(ctx, id)` — `COUNT(*)` of purchase_requests with this `cost_center_id`
  (mirrors `GetVendorUsage`), shown on the detail page before deactivating.
- Active-user directory already exists: reuse `ListActiveUsers` for owner pickers.

### 3. Auth helper — `backend/internal/middleware/auth.go`
Add `HasCostCenterAdmin(ctx)` returning `HasRole(RoleFinanceAdmin) || HasRole(RoleAdmin)` — same
body as [`HasVendorAdmin`](purchasing-app/backend/internal/middleware/auth.go#L218). (Kept as its
own named helper for clarity rather than reusing the vendor one.)

### 4. Handler — `backend/internal/handler/cost_centers.go` (new)
Copy the shape of [vendors.go](purchasing-app/backend/internal/handler/vendors.go):
`CostCentersHandler{Repo, Log}`, a `costCenterInput` JSON struct with `toRepo()` (trim strings),
and methods:
- `Lookup`  — any authenticated user → `ListActiveCostCenters`. (No admin gate.)
- `List` / `Get` / `Create` / `Update` / `Usage` — all gated by `middleware.HasCostCenterAdmin`,
  403 otherwise. `Create`/`Update` validate `name != ""`. `Update` carries `is_active`, so
  deactivate/reactivate is just a PUT (same as vendors — no separate delete endpoint).

### 5. Routes — `backend/internal/handler/router.go`
Construct `costCenters := &CostCentersHandler{Repo: d.Repo, Log: d.Log}` and register beside vendors:
```go
r.Get("/cost-centers/lookup", costCenters.Lookup)   // any authenticated user
r.Get("/cost-centers", costCenters.List)
r.Post("/cost-centers", costCenters.Create)
r.Get("/cost-centers/{id}", costCenters.Get)
r.Put("/cost-centers/{id}", costCenters.Update)
r.Get("/cost-centers/{id}/usage", costCenters.Usage)
```
Register `/cost-centers/lookup` **before** `/cost-centers/{id}` so it isn't captured as an id.

### 6. PR linkage — `repository.go` + `purchase_requests.go`
- Add `CostCenterID *int64` to the `PurchaseRequest` read struct and the PR input struct
  ([repository.go:326,357](purchasing-app/backend/internal/repository/repository.go#L326)); add
  `cost_center_id` to the INSERT/UPDATE (lines 377/408) and SELECT/scan (lines 449/493). Join
  `cost_centers` to also return `cost_center` (name) for display so existing list/detail views keep
  working. On create/update, mirror the selected cost center's name into the legacy `cost_center`
  text column for continuity.
- Add `cost_center_id` to `prInput` in
  [purchase_requests.go:50](purchasing-app/backend/internal/handler/purchase_requests.go#L50).

---

## Frontend (React + TS)

### 7. Types — `frontend/src/types/api.ts`
Add (mirroring `Vendor`/`VendorInput` and the `vendorRef`/`formatMoney` helpers):
- `CostCenter`, `CostCenterInput`, `CostCenterSummary`, `CostCenterUsage`.
- `emptyCostCenter` const and `costCenterRef(id) => "CC-" + pad6`.
- Add `cost_center_id?: number | null` to `PurchaseRequest` and `PurchaseRequestInput`.

### 8. API client — `frontend/src/api/costCenters.ts` (new)
Copy [vendors.ts](purchasing-app/frontend/src/api/vendors.ts): `listCostCenters`, `getCostCenter`,
`getCostCenterUsage`, `createCostCenter`, `updateCostCenter`, plus
`lookupCostCenters() => apiFetch<CostCenterSummary[]>("/api/v1/cost-centers/lookup")`.

### 9. Hooks — `frontend/src/hooks/`
- `useCostCenters` / `useCostCenterLookup` (model [useVendors](purchasing-app/frontend/src/hooks/useUserLookup.ts)).
- `useCanManageCostCenters` — `admin || finance_admin`, a copy of
  [useCanManageVendors.ts](purchasing-app/frontend/src/hooks/useCanManageVendors.ts).

### 10. Form component — `frontend/src/components/CostCenterFields.tsx` (new)
Model [VendorFields.tsx](purchasing-app/frontend/src/components/VendorFields.tsx). Inputs: code,
name, description (textarea), budget (`type=number step="any"`) + currency, and `is_active`.
- **Primary owner**: single `<select>` of active users from `useUserLookup` (blank = none).
- **Secondary owners**: reuse [ApproverPicker.tsx](purchasing-app/frontend/src/components/ApproverPicker.tsx)
  directly (multi-select chips). Filter the primary owner out of the secondary candidate list.

### 11. Pages
- `frontend/src/pages/CostCenterListPage.tsx` (new) — copy
  [VendorListPage.tsx](purchasing-app/frontend/src/pages/VendorListPage.tsx): permission guard via
  `useCanManageCostCenters`, inline add form, search, active/inactive/all filter, table linking to
  detail (`CC-…` ref).
- `frontend/src/pages/CostCenterDetailPage.tsx` (new) — copy `VendorDetailPage.tsx`: view/edit
  toggle, Deactivate/Reactivate button, and a "where used" count from `getCostCenterUsage`.

### 12. Routing & nav
- `frontend/src/App.tsx`: import the pages, add `/cost-centers` and `/cost-centers/:id` routes
  beside the vendor routes ([App.tsx:63-64](purchasing-app/frontend/src/App.tsx#L63-L64)).
- `frontend/src/components/Layout.tsx`: add a `Cost centers` nav link gated by
  `useCanManageCostCenters`, beside the Vendors link
  ([Layout.tsx:56-60](purchasing-app/frontend/src/components/Layout.tsx#L56-L60)).

### 13. PR form — replace free-text with dropdown
In [RequestFields.tsx:37-44](purchasing-app/frontend/src/components/RequestFields.tsx#L37-L44),
replace the text input with a `<select>` populated from `useCostCenterLookup`, bound to
`value.cost_center_id`. Keep it optional. (Existing PRs with legacy text-only cost centers still
display their stored name.)

---

## Verification

1. **Migrate:** `psql -h localhost -d purchasing -f backend/migrations/018_cost_centers.sql`.
2. **Build/run:** `cd backend && go build ./... && go run ./cmd/server`; `cd frontend && npm run dev`.
3. **RBAC:** sign in as `staff` → no "Cost centers" nav link; hitting `/cost-centers` shows the
   permission-denied panel; `GET /api/v1/cost-centers` returns 403 but `/cost-centers/lookup` works.
   Sign in as `admin`/`finance_admin` → full access.
4. **CRUD:** create a cost center (name only), then one with code/owners/budget; edit it; deactivate
   and confirm it drops from the active filter and the PR dropdown; reactivate.
5. **Owners:** set a primary owner and 2 secondary owners; reload detail and confirm they persist;
   confirm the primary owner can't also be added as secondary.
6. **PR linkage:** create a PR, pick a cost center from the dropdown, save, reload the PR detail and
   confirm the cost center name shows; deactivated centers don't appear in the dropdown.
7. **Usage guard:** open the detail page of a cost center referenced by a PR and confirm the
   "where used" count is non-zero before deactivating.
8. **Tests:** `cd backend && go test ./...` (the integration test in
   `repository_integration_test.go` sets `CostCenter` — update if the PR input shape changes).
