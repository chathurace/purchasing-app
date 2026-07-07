# Vendor Management Page

## Context

Vendors currently have no home of their own. The `vendors` table and a basic
CRUD API exist (List/Create/Get/Update), but the only UI is the inline
`VendorSelect` "+ New vendor" form used while entering a quotation. There is no
way to browse, edit, or retire vendors, and the record is minimal (name,
contact, email, phone, notes).

We want a dedicated **Vendor Management** page, restricted to `admin` and
`finance_admin` roles, that gives finance leads a proper vendor master: browse +
search, create, edit, deactivate, and see where each vendor is used. This mirrors
the basic "vendor master" found in typical ERP systems, scoped down to essentials.

### Decisions (confirmed with user)
- **Extra fields**: add **Active/Inactive status**, **address** (basic structured
  fields), and **tax / registration ID**. (No payment terms yet.)
- **Lifecycle**: **deactivate only** — no hard delete (FK `ON DELETE RESTRICT`
  protects referenced vendors anyway). Inactive vendors are hidden from new
  selection but keep history.
- **Inline creation stays**: regular `finance` users keep the quick "+ New vendor"
  inline form during quotation entry. Only the dedicated page (edit / deactivate /
  full record) is gated to `admin` + `finance_admin`.
- **Where-used**: vendor detail shows counts + links to related quotations,
  contracts, GRNs, and invoices.

## Access model

Add a helper alongside the existing ones in
[middleware/auth.go](purchasing-app/backend/internal/middleware/auth.go):

```go
// HasVendorAdmin gates the dedicated vendor management actions.
func HasVendorAdmin(ctx context.Context) bool {
    return HasRole(ctx, model.RoleFinanceAdmin) || HasRole(ctx, model.RoleAdmin)
}
```
(`admin` already passes every `HasRole` check.)

Gating per endpoint:
- `GET /vendors`, `GET /vendors/{id}`, `POST /vendors` — stay at `HasFinanceAccess`
  (needed by `VendorSelect` + dropdown for finance users).
- `PUT /vendors/{id}` (full edit incl. status) and the new usage endpoint —
  tighten to `HasVendorAdmin`.

Frontend mirrors this with a new hook `useCanManageVendors()` returning
`roles.includes("admin") || roles.includes("finance_admin")`, modeled on
[useFinanceAccess.ts](purchasing-app/frontend/src/hooks/useFinanceAccess.ts) /
[useIsAdmin.ts](purchasing-app/frontend/src/hooks/useIsAdmin.ts).

## Backend changes

### 1. Migration `backend/migrations/016_vendor_management.sql`
Extend the `vendors` table (next number after `015`):
```sql
ALTER TABLE vendors
    ADD COLUMN is_active     BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN tax_id        TEXT    NOT NULL DEFAULT '',
    ADD COLUMN address_line  TEXT    NOT NULL DEFAULT '',
    ADD COLUMN city          TEXT    NOT NULL DEFAULT '',
    ADD COLUMN postal_code   TEXT    NOT NULL DEFAULT '',
    ADD COLUMN country       TEXT    NOT NULL DEFAULT '';
CREATE INDEX idx_vendors_active ON vendors(is_active);
```
Apply with `psql -h localhost -d purchasing -f backend/migrations/016_vendor_management.sql`.

### 2. Repository
[repository/procurement.go](purchasing-app/backend/internal/repository/procurement.go) (vendor block ~lines 32–110):
- Add the new fields to `Vendor` struct and `VendorInput`.
- Update `CreateVendor`, `UpdateVendor`, `ListVendors`, `GetVendor` SQL to
  include the new columns. `UpdateVendor` writes `is_active` too (activate/
  deactivate = an update with the toggled flag).
- New method `GetVendorUsage(ctx, id) (VendorUsage, error)` returning counts via
  4 `SELECT COUNT(*)` queries (or one `UNION ALL`) against `quotations`,
  `contracts`, `grns`, `invoices` where `vendor_id = $1`.

### 3. Handler
[handler/vendors.go](purchasing-app/backend/internal/handler/vendors.go):
- Add new fields to the `vendorInput` struct + `toRepo()` (trim strings).
- `Create` defaults new vendors to active (handled by DB default).
- `Update`: change the guard to `middleware.HasVendorAdmin`; accept `is_active`.
- New handler `Usage` (gated `HasVendorAdmin`) → `Repo.GetVendorUsage`.

### 4. Router
[handler/router.go](purchasing-app/backend/internal/handler/router.go) (vendor block ~lines 64–68):
add `r.Get("/vendors/{id}/usage", vendors.Usage)`.

## Frontend changes

### Types & API
- [types/api.ts](purchasing-app/frontend/src/types/api.ts): add `is_active`,
  `tax_id`, `address_line`, `city`, `postal_code`, `country` to `Vendor` and
  `VendorInput`. Add a `VendorUsage` interface. `vendorRef()` already exists.
- [api/vendors.ts](purchasing-app/frontend/src/api/vendors.ts): add
  `getVendorUsage(id)`. Reuse existing `listVendors`/`getVendor`/`createVendor`/
  `updateVendor`.
- Hooks: keep [useVendors.ts](purchasing-app/frontend/src/hooks/useVendors.ts);
  add `useVendor(id)`, `useVendorUsage(id)`, and `useCanManageVendors()`.

### Pages (new) — pattern from [UserManagementPage.tsx](purchasing-app/frontend/src/pages/UserManagementPage.tsx)
- **`pages/VendorListPage.tsx`**: top-of-page role guard (show "You need admin or
  finance_admin role…" message if not, like `UserManagementPage`); name search
  box + active/inactive filter; table showing `VEN-NNNNNN`, name, contact, status
  badge; inline "Add vendor" form (reuse `VendorFields`); rows link to detail.
- **`pages/VendorDetailPage.tsx`**: same guard; edit form (`VendorFields`); an
  Activate/Deactivate button (calls `updateVendor` with toggled `is_active`); a
  **Where used** panel rendering `useVendorUsage` counts as links to filtered
  list pages (or just counts initially).
- **`components/VendorFields.tsx`**: reusable field group (name, contact, email,
  phone, tax id, address fields, notes) shared by list inline-add and detail edit,
  modeled on existing `*Fields.tsx` components.

### Wiring
- [App.tsx](purchasing-app/frontend/src/App.tsx): add
  `<Route path="/vendors" element={<VendorListPage />} />` and
  `<Route path="/vendors/:id" element={<VendorDetailPage />} />`.
- [Layout.tsx](purchasing-app/frontend/src/components/Layout.tsx): add a
  `Vendors` nav link gated by `useCanManageVendors()` (place near Users).
- [VendorSelect.tsx](purchasing-app/frontend/src/components/VendorSelect.tsx):
  filter the dropdown to `is_active` vendors only, but keep the currently selected
  vendor visible even if inactive (so existing quotations still render their
  vendor). Inline create is unchanged.

## Verification

1. Apply migration: `psql -h localhost -d purchasing -f backend/migrations/016_vendor_management.sql`.
2. Run backend (`cd backend && go run ./cmd/server`) and frontend
   (`cd frontend && npm run dev`).
3. **As `admin` or `finance_admin`**: the **Vendors** nav link appears →
   create a vendor with the new fields → edit it → deactivate it (confirm it
   shows as inactive and drops out of the `VendorSelect` dropdown) → reactivate →
   open detail and confirm the **Where used** counts match existing
   quotations/contracts for a referenced vendor.
4. **As a plain `finance` user**: no Vendors nav link; navigating directly to
   `/vendors` shows the access message; but the inline "+ New vendor" during
   quotation entry still works.
5. **As `staff`**: no vendor access anywhere (unchanged).
6. Backend authz check: `PUT /api/v1/vendors/{id}` and `/usage` return 403 for a
   `finance`-only token; `GET`/`POST /vendors` still succeed.
7. `cd frontend && npm run build` (typecheck) and `cd backend && go build ./...`.

## Notes
- Per repo convention, before approval copy this plan to
  `docs/plans/NN-vendor-management.md` (e.g. `04-vendor-management.md`).
