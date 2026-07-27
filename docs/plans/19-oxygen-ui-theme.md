# Migrate purchasing-app webapp to the finops (WSO2 Oxygen UI) theme

## Context

The purchasing-app webapp is currently a **Tailwind CSS v3** SPA (React 18) with an indigo
brand, a light-only look, a horizontal top-nav shell, and one isolated hand-written CSS island
for the ProQ requisition wizard (`.proq-req`, blue).

The sibling **finops-app** webapp does **not** use Tailwind at all — its entire look comes from
the WSO2 **Oxygen UI** design system (`@wso2/oxygen-ui`, a MUI v6 + Emotion wrapper): brand
**orange `#FF7300`** + secondary blue `#5CD1FF`, Inter Variable, very rounded corners
(radius 20), flat/no-shadow surfaces, translucent glassmorphism (`backdrop-blur`), and radial
orange/purple background washes. Its shell is a slim top **Header** + a collapsible left
**Sidebar** (`AppShell`).

**Goal:** make purchasing-app look like finops-app by doing a **full migration** off Tailwind
onto Oxygen UI/MUI — theme, shell, and every page/component rewritten with MUI components +
`sx`. Decisions confirmed with the user:
- **Full framework migration** to `@wso2/oxygen-ui` (not a Tailwind re-skin).
- **Full end-to-end** execution (all ~55 files, Tailwind removed, then verified).
- **Adopt the finops side-nav shell** (AppShell + Header + collapsible Sidebar).
- **Light-only for now** (no dark/system toggle), even though the theme ships dark.
- **Recolor the ProQ requisition form** (`RequisitionForm.css`) from blue to WSO2 orange.

`@wso2/oxygen-ui` is on **public npm** (0.13.0 latest; finops pins 0.12.0 — we will pin
**0.12.0** to match finops exactly). Its peer deps pin **React 19.2.3**, so this migration also
requires a **React 18.3 → 19.2.3 upgrade**.

## Dependency changes (`webapp/package.json`)

Mirror finops's proven set:
- **Upgrade**: `react` & `react-dom` `^18.3.1` → `19.2.3`; `@types/react` → `^19.2.7`,
  `@types/react-dom` → `^19.2.3`. (react-router-dom 6.28, @tanstack/react-query 5.62, and
  oidc-client-ts 3.1 are all React-19-compatible — finops runs the same families on 19.)
- **Add**: `@wso2/oxygen-ui@0.12.0`, `@wso2/oxygen-ui-icons-react@0.12.0`,
  `@emotion/react@^11.14.0`, `@emotion/styled@^11.14.1`.
- **Remove (final phase)**: `tailwindcss`, `autoprefixer`, `postcss`; delete
  `tailwind.config.js` and `postcss.config.js`.
- Charts (`@wso2/oxygen-ui-charts-react`) are **not** needed — purchasing has no charts.

## Migration strategy

Tailwind and MUI can coexist during the conversion, but Tailwind's preflight fights MUI's
`CssBaseline`. Because this is a full end-to-end pass, the cleanest path is: stand up the Oxygen
foundation, convert every file, and **remove Tailwind last** — keeping it installed only so
unconverted files still render mid-pass. The `.proq-req` CSS island is plain scoped CSS (not
Tailwind), so it survives Tailwind removal untouched and only needs recoloring.

### Phase 1 — Foundation (theme + shell)
- **`src/main.tsx`**: wrap the tree in `<OxygenUIThemeProvider theme={WSO2Theme}>` (copy the
  finops wiring). Lock to **light-only**: pass the provider's light default (verify the exact
  prop — `defaultMode="light"` / forcing `document.documentElement` `data-color-scheme="light"`)
  and render **no** ColorSchemeToggle. Keep importing a global CSS file.
- **`src/App.css`** (new): port finops's light-mode radial background wash (orange + purple
  radials over `--oxygen-palette-background-default`) + `#root { min-height:100vh }` + inherited
  form fonts + themed thin scrollbars.
- **`src/components/Layout.tsx`**: replace the top-nav `<header>`/`<nav>` with an Oxygen
  **`AppShell`** (`AppShell.Navbar` → `Header` with brand "Purchasing" + `UserMenu`;
  `AppShell.Sidebar` → collapsible `Sidebar` with `Header.Toggle`; `AppShell.Main` wrapping
  `<Outlet/>` in a full-width `Box`). Model on finops `components/shell/Shell.tsx`, including the
  mobile overlay `Drawer` at the `md` breakpoint.
- **`src/lib/nav/navModel.ts`** (new): a `getNavGroups(perms)` pure function encoding
  purchasing's existing role-gated nav (Home, My requests, Purchase requests [procurement],
  Approvals, Quotations/Contracts [canApprove], GRNs/Invoices [procurement], Vendors, Business
  units, Users [admin], Settings), using `@wso2/oxygen-ui-icons-react` icons. Reuse the existing
  permission hooks (`useProcurementAccess`, `useIsAdmin`, `useIsApprover`,
  `useCanManageVendors`, `useCanManageBusinessUnits`).
- **`UserMenu`**: rebuild the current hand-rolled dropdown (in `Layout.tsx`) as an Oxygen
  menu/avatar in the Header.
- **`src/pages/LoginPage.tsx`** and **`CallbackPage.tsx`**: re-theme with MUI (they render
  outside the shell). Replace the indigo-gradient logo badge with the orange brand.
- **`index.html`**: title stays "Purchasing"; drop the Google Fonts `<link>` (Inter Variable
  ships inside Oxygen). Space Grotesk / IBM Plex Mono links stay **only** if the ProQ form still
  needs them (it does — keep them).

### Phase 2 — Shared components → MUI
Convert the reusable primitives so pages inherit a consistent look:
- **`StatusBadge.tsx`, `EntityStatusBadge.tsx`** → MUI `Chip` with semantic palette colors
  (success/warning/error/info) replacing the hardcoded Tailwind pill maps.
- Form + display primitives used everywhere: `ApproverPicker`, `ConfirmDialog`, `DocumentList`,
  `RecordCard`, `CurrencyInput`, `NumberInput`, `EmailAutocomplete`, `DirectoryUserPicker`,
  `UserComboBox`, `SupplierNameCombobox`, `VendorSelect`, `ChainStepper`, field groups
  (`InvoiceFields`, `GRNFields`, `QuotationFields`, `ContractFields`, `VendorFields`,
  `BusinessUnitFields`), section/card wrappers (`CaseSections`, `RecommendationSection`,
  `ContractContent`, `ContractFulfillment`, `AssignmentCard`, `TeamLeadApprovalCard`,
  `TeamsSection`, `StorageSettings`, `RelatedDocuments`, `PurchaseRequestsList`,
  `VendorFilterNotice`) → MUI (`TextField`, `Select`, `Autocomplete`, `Button`, `Card`,
  `Dialog`, `Stack`, `Box`, `Table`). Pattern: replace `.app-card`/`.btn-*`/`.field`/`.badge`
  and inline utility strings with MUI components + `sx` reading theme tokens
  (`bgcolor:'background.paper'`, `borderColor:'divider'`, etc.), per finops's
  `Card variant="outlined"` + `size="small"` inputs convention.

### Phase 3 — Pages → MUI (~24 pages)
Convert each page in `src/pages/` to the finops page pattern: a
`<Box sx={{ maxWidth: 1200, mx:'auto', p:{xs:2, md:4} }}>` content container, MUI cards/tables,
`Alert` for errors. Pages: Home, My requests, Purchase request list/detail, New PR, Approvals,
Quotation list/detail, Contract list/detail, GRN list/detail + new, Invoice list/detail + new,
Vendor list/detail, Business unit list/detail, Users, Settings.

### Phase 4 — ProQ requisition form recolor
`src/components/RequisitionForm.css` stays a scoped `.proq-req` CSS island (survives Tailwind
removal) but its palette is retuned blue → WSO2 orange: `--proq-blue #0b5cff → #FF7300`,
`--proq-blue-deep #0847c4 → #d96200` (darker orange), `--sky-tint #eaf1ff → a warm orange
tint`, and the hero/stepper/`.btn-submit` gradients to orange. Keep the layout, Space Grotesk /
IBM Plex Mono fonts, and structure. `RequisitionForm.tsx` needs no logic change.

### Phase 5 — Remove Tailwind
- Delete `tailwind.config.js`, `postcss.config.js`.
- Rewrite `src/index.css` to drop `@tailwind`/`@layer` (fold anything still needed into
  `App.css`), or remove it and rely on `App.css` + Oxygen `CssBaseline`.
- Uninstall `tailwindcss`/`autoprefixer`/`postcss`.
- Grep `src/` for leftover `className=` utility strings and stray `.app-card`/`.btn-`/`.field`/
  `.badge` references; convert any stragglers.

## Key reference files (source of truth in finops)
- Theme wiring: `.../finops-app/webapp/src/main.tsx`
- Shell: `.../finops-app/webapp/src/components/shell/Shell.tsx` (+ `PageHeader.tsx`)
- Nav model: `.../finops-app/webapp/src/lib/nav/navModel.ts`
- Page pattern: `.../finops-app/webapp/src/pages/Home.tsx`
- Global CSS: `.../finops-app/webapp/src/App.css`
- Token values already extracted (orange `#FF7300`, secondary `#5CD1FF`, semantic colors,
  radius 20, flat shadows, blur tokens) — in this session's exploration notes.

## Verification
1. `cd webapp && npm install` — confirm React 19 + Oxygen resolve with no peer-dep errors.
2. `npm run typecheck` — clean (React 19 types; MUI props).
3. `npm run build` — production build succeeds.
4. `npm run dev` → open http://localhost:5173, sign in, and click through the app driving the
   real flows (use the `run`/`verify` skills): Login, Home dashboard, side-nav (expand/collapse
   + mobile drawer), a PR list + detail, the **new-PR ProQ wizard** (confirm it's orange), a
   quotation/contract/invoice page, Settings, Users. Confirm: orange brand throughout, glass
   surfaces, background wash, no indigo remnants, no broken Tailwind classes, and the app is
   locked to light mode.
5. Backend unaffected — no Go or migration changes.

## Risks / notes
- **React 19 upgrade** is the main risk, but finops proves the exact dependency combination
  works on 19; ranges in purchasing already allow it.
- Oxygen pinned to **0.12.0** to match finops (0.13.0 exists but stays untested here).
- Interim mid-pass renders may show Tailwind/MUI reset clashes until Phase 5; acceptable since
  removal is part of the same end-to-end pass.
- No backend, migration, or API changes — this is webapp-only.
