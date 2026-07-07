# In-app Google Drive storage configuration (admin settings)

## Context

Today, pointing the app at a Google Drive folder requires editing `config.yaml`,
running two CLI tools (`cmd/gdrive-grant` for the Picker grant, `cmd/gdrive-auth`
for the refresh token), and restarting the server. That's fine for a developer but
unusable for a non-technical admin on a real deployment.

Goal: an admin configures Drive storage entirely from **Settings → File storage** —
"Connect Google account" (OAuth), "Choose folder" (Picker), see live status — with
the running store **hot-swapped** (no restart). Per decisions: hot-swap, **no**
existing-file migration (warn only), and the **CLI tools stay** as a
bootstrap/headless fallback.

The heavy lifting already exists: `storage.NewGDriveStoreOAuth` (auth via refresh
token) and the browser Picker+GIS flow (embedded in `cmd/gdrive-grant`). This work
moves that flow into the app and makes storage config runtime-mutable.

## Key insight that keeps the change small

`Deps.Storage` is a `storage.Store` handed to each resource handler
([router.go](backend/internal/handler/router.go) lines 33–39) and to the
`documents.go` helpers. If we introduce a **`StorageManager` that itself
implements `storage.Store`** (delegating to the current inner store under an
`RWMutex`), then **no handler changes are needed** — they keep calling
`h.Storage.Save/Open/Delete`, which now routes through the swappable manager.

## Design

### 1. `StorageManager` (backend, new) — `backend/internal/storage/manager.go`
- Implements `storage.Store`; holds current `Store` behind `sync.RWMutex`;
  `Save/Open/Delete` forward to it.
- `Reconfigure(ctx, settings) error`: builds a new store (reusing
  `NewGDriveStoreOAuth` / `NewLocalStore`), validates it (base-folder resolve +
  `canAddChildren`), then atomically swaps. On failure keeps the old store and
  returns the error — a bad config never takes storage down.
- `Status()`: backend, base folder id + name, connected account email,
  healthy/last-error — for the settings UI.

### 2. Config split: static (config.yaml) vs dynamic (DB)
- **Static, per-deployment GCP artifacts** stay in `config.yaml`, added to
  [config.go](backend/internal/config/config.go) under `storage.gdrive.oauth`:
  `api_key` (browser Picker key, new) and `app_id` (new, optional; defaults to the
  client-id prefix). `client_id`/`client_secret` already exist. Plus a new
  `security.secret_key` (32-byte base64) to encrypt secrets at rest.
- **Dynamic, per-admin action** moves to a DB row (`storage_settings`):
  `backend`, `base_folder_id`, `base_folder_name`, `google_account_email`,
  `refresh_token_enc` (encrypted), `updated_by`, `updated_at`.
- **Precedence at startup** (`main.go`): if a DB row exists → use it (decrypt
  token); else fall back to the `config.yaml` oauth block. This keeps existing
  deployments and the CLI flow working; an in-app save writes the DB and takes
  over.

### 3. Secrets at rest — `backend/internal/crypto/secret.go` (new)
AES-256-GCM `Encrypt/Decrypt` keyed by `security.secret_key`. Only
`refresh_token` is user-supplied and stored in the DB, so only it is encrypted;
`client_secret`/`api_key` remain ops-managed in `config.yaml`. (No crypto helper
exists today — this is new but ~40 lines.)

### 4. Browser OAuth + Picker (frontend)
- **Connect account**: GIS **code client** (`initCodeClient`, scope `drive.file`,
  `ux_mode:'popup'`, `redirect_uri:'postmessage'`, offline) returns an auth code to
  JS → POST `/storage/connect` → backend exchanges it for a refresh token, encrypts
  and stores it, records the account email. (`postmessage` avoids needing a server
  redirect route; the app origin must be an Authorized JS origin on the client.)
- **Pick folder**: port the Picker logic from `cmd/gdrive-grant`'s embedded HTML
  (My Drive + Shared Drive tabs, `setAppId`) into a React component. It pulls
  `client_id`/`api_key`/`app_id` from `/storage/params` and a token from the GIS
  token client. On pick → POST `/storage/folder {id,name}` → backend validates +
  hot-swaps.

### 5. Backend endpoints — `backend/internal/handler/storage.go` (new)
Admin-only (reuse the `requireAdmin` pattern from
[users.go](backend/internal/handler/users.go) line 48 / `middleware.HasRole`,
`RoleAdmin`), mounted in the admin group of `router.go`:
- `GET  /api/v1/storage/status`  → `Status()`.
- `GET  /api/v1/storage/params`  → `{client_id, api_key, app_id, connected, account_email}` (non-secret).
- `POST /api/v1/storage/connect` `{code}` → exchange → store token → status.
- `POST /api/v1/storage/folder`  `{base_folder_id, name}` → validate + persist + hot-swap.

### 6. Repository + migration
- `backend/internal/repository/storage_settings.go` (new): `GetStorageSettings`,
  `UpsertStorageSettings` (follow `config_options.go` style).
- `backend/migrations/033_storage_settings.sql`: single-row `storage_settings`
  table (id PK check =1, columns per §2).

### 7. Frontend files
- `frontend/src/api/storage.ts` (new): `getStatus`, `getParams`, `connect(code)`,
  `setFolder(id,name)` via `apiFetch`.
- `frontend/src/hooks/useStorage.ts` (new): TanStack Query wrappers.
- Extend [SettingsPage.tsx](frontend/src/pages/SettingsPage.tsx) with an
  **admin-only** "File storage" section (page is already admin/finance_admin; gate
  this section to `admin`). A small `GoogleDrivePicker` component loads the Google
  JS (`api.js` + `gsi/client`) dynamically. UI = Connect → Choose folder → status
  card, with a persistent warning: *"Changing the folder does not move existing
  files; documents stored in the previous folder will no longer be downloadable."*

### 8. CLI tools & GCP delta
- `cmd/gdrive-grant` / `cmd/gdrive-auth` unchanged — they still emit `config.yaml`
  values, now serving as the bootstrap/headless fallback (used when the DB row is
  empty). Document the relationship in `docs/file-storage.md`.
- Ops (once): add the deployed app origin (+ `http://localhost:5173` for dev) to
  the Web OAuth client's Authorized JS origins; ensure an API key exists. The
  `localhost:8899` loopback remains only for the CLI tools.

## Assumptions (baked into the plan; say if you disagree)
- `client_secret`, `api_key`, and `security.secret_key` live in `config.yaml`
  (ops-managed), not entered in the UI.
- v1 is Drive-focused; no "switch to local backend" toggle in the UI.
- Extend the existing `SettingsPage` rather than add a separate route.

## Files to create / modify
- New: `internal/storage/manager.go`, `internal/crypto/secret.go`,
  `internal/repository/storage_settings.go`, `internal/handler/storage.go`,
  `migrations/033_storage_settings.sql`, `frontend/src/api/storage.ts`,
  `frontend/src/hooks/useStorage.ts`, a `GoogleDrivePicker` component.
- Modify: `internal/config/config.go`, `cmd/server/main.go`,
  `internal/handler/router.go` (+ `Deps`), `frontend/src/pages/SettingsPage.tsx`,
  `docs/file-storage.md`; archive plan to `docs/plans/10-inapp-gdrive-settings.md`.

## Verification
- **Backend**: `go build ./... && go vet ./...`; unit tests for
  `crypto.Encrypt/Decrypt` round-trip and `StorageManager.Reconfigure`
  (swaps on valid, keeps old + errors on invalid).
- **End-to-end** (backend + `npm run dev`, as admin):
  1. Settings → File storage → **Connect** (Google consent) → account shows.
  2. **Choose folder** (Picker) → status card healthy.
  3. Attach a quotation to a PR → file lands in the chosen folder (per-PR subfolder
     appears in Drive).
  4. Pick a **different** folder → confirm hot-swap **without restart**; new uploads
     go to the new folder; the migration warning is shown.
- **Fallback**: empty DB + `config.yaml` oauth block → server still boots (CLI path
  intact).
- **Token loss**: revoke at myaccount.google.com → status flips to
  unhealthy/"reconnect".
