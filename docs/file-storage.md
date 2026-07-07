# File storage

Document files (quotations, contracts, GRNs, invoices, signed PDFs, RFI/comment
attachments) are stored through a single `storage.Store` interface with two
interchangeable backends. `storage.backend` in `config.yaml` selects the backend;
for the Drive backend the *account and folder* are runtime-mutable (see
[Runtime configuration](#runtime-configuration)). All handlers touch files only
through the three helpers in `internal/handler/documents.go`, which call
`Store.Save`/`Open`/`Delete` — so the backend is invisible above the storage
package.

```go
// internal/storage/store.go
type Store interface {
	Save(prID int64, originalName string, r io.Reader) (relPath string, size int64, err error)
	Open(relPath string) (io.ReadCloser, error)
	Delete(relPath string) error
}
```

`Save` returns an **opaque token** persisted verbatim in `documents.stored_path`
and handed back to `Open`/`Delete`. Each backend defines its own token format;
callers never parse it.

## Backends

### `local` (default) — `internal/storage/local.go`

Files on the local filesystem under `storage.files_root`, laid out as
`<root>/purchase-requests/<prID>/<docUUID>/<originalFilename>`. The per-document
UUID folder avoids filename collisions. `stored_path` is the root-relative path,
e.g. `purchase-requests/42/1a2b.../quote.pdf`.

### `gdrive` — `internal/storage/gdrive.go`

Files in a Google Drive folder (My Drive or a Shared Drive), laid out as
`<base_folder_id>/<prID>/<originalFilename>` — one subfolder per PR (named by PR
id), files flat inside it. `stored_path` is `gdrive:<fileID>`; the `gdrive:`
prefix makes rows self-describing and makes a mis-routed read fail loudly rather
than treating a Drive ID as a filesystem path.

Two authentication modes (`storage.gdrive.auth`):

- **`oauth` (recommended)** — the app acts as a **real user** via an OAuth refresh
  token (`NewGDriveStoreOAuth`), with the fine-grained **`drive.file`** scope. The
  app can reach only files it creates plus the one base folder the user grants it
  via the Google Picker — nothing else in the user's Drive. `drive.file` is a
  *non-sensitive* scope (no restricted-scope verification). Because the user has
  personal Drive quota and inherits their own access to Shared Drives, this works
  where org policy blocks adding a service account to a Shared Drive.
- **`service_account`** — the app acts as itself via a service-account JSON key,
  with the full **`drive`** scope (needed to read a human-created folder). No
  in-app configuration; folder is set in `config.yaml`.

Design notes:
- **Service-account quota trap**: plain service accounts have **no personal Drive
  storage quota**, so uploads to a My Drive folder fail with
  `storageQuotaExceeded` — the SA can *read* a shared My Drive folder but can't
  *own* files there. A service account must therefore write into a **Shared
  Drive** (org-owned storage). `oauth` mode sidesteps this entirely (files are
  owned by the user / the Shared Drive the user belongs to).
- **Picker grant needs the App ID**: for `drive.file`, selecting a folder in the
  Picker only grants the app access when the Picker is built with
  `setAppId(<project number>)`. Without it, reads of the picked folder 404 even
  though the Picker succeeded. Both `cmd/gdrive-grant` and the in-app picker set it
  (the app id defaults to the client-id prefix).
- **Startup/reconfig validation**: `newGDriveStore` resolves the base folder once,
  failing fast if the authenticated identity can't see it (a 404 means "not shared
  with / not granted to this identity") or if it's not writable (`canAddChildren`).
- Every Drive call sets `SupportsAllDrives(true)`; List calls also set
  `IncludeItemsFromAllDrives(true)` and scope `Corpora("drive").DriveId(...)`.
- **Folder find-or-create** is guarded by a mutex + an in-process cache
  (`prID → folderID`), preventing duplicate `<prID>` folders under concurrent
  uploads and skipping a List round-trip on repeat uploads. Per-process guarantee;
  if duplicates ever exist, the first match wins.
- Duplicate filenames within a PR folder are fine — Drive addresses files by ID.
- The interface is context-free (matching `LocalStore`'s `os` calls); the Drive
  backend uses internal timeouts (60s metadata, 10 min blob transfers; a
  download's context is tied to the returned body's `Close()`).

## Runtime configuration

The `Store` handed to handlers is a **`StorageManager`**
(`internal/storage/manager.go`) that itself implements `Store` and forwards to a
current inner store behind an `RWMutex`. This lets an admin reconfigure Drive
**without a restart**: a new config is built, validated (base-folder resolve +
`canAddChildren`), then swapped in atomically; a bad config is rejected and the
previous store is kept — a misconfiguration never takes storage down. When storage
is not configured yet, `Save/Open/Delete` return `ErrNotConfigured` and the server
still boots (surfaced as unhealthy status) rather than aborting at startup.

Config is split in two:

- **Static** GCP artifacts live in `config.yaml`: `client_id`, `client_secret`,
  `api_key`, optional `app_id`, and `security.secret_key`.
- **Dynamic** config — which folder + which account's refresh token — lives in the
  `storage_settings` table (single row, migration `033`), written when an admin
  configures storage from the UI. The refresh token is encrypted at rest with
  AES-256-GCM (`internal/crypto`, keyed by `security.secret_key`).

**Precedence at startup**: a saved `storage_settings` row wins; otherwise the
`config.yaml` `oauth`/`service_account` block is the bootstrap/fallback (the CLI
path). An admin saving from the UI writes the DB row and takes over.

Admin-only endpoints (`internal/handler/storage.go`): `GET /storage/status`,
`GET /storage/params` (non-secret bits for the browser), `POST /storage/connect`
(exchange auth code → encrypt + store refresh token), `POST /storage/folder`
(validate + hot-swap).

## Configuration (`config.yaml`)

```yaml
storage:
  backend: "gdrive"            # "local" or "gdrive"
  files_root: "../files"       # local backend only
  gdrive:                      # required when backend: "gdrive"
    # --- oauth (recommended, acts as a real user; drive.file scope) ---
    auth: "oauth"
    oauth:
      client_id: "<oauth-web-client-id>"      # static GCP artifacts — enable the
      client_secret: "<oauth-web-client-secret>"  # in-app connect + Picker flow
      api_key: "<google-api-key>"             # Picker developer key
      app_id: ""                              # optional; default = client_id prefix
      # base_folder_id / refresh_token optional — set only for a CLI bootstrap.
    # --- service_account (alternative; full drive scope, Shared Drive only) ---
    # auth: "service_account"
    # credentials_file: "./gdrive-sa.json"    # JSON key — gitignore it
    # base_folder_id: "<shared-drive-folder-id>"

# Encrypts the refresh token stored via the in-app settings. Required once Drive
# is connected from the UI. Generate: head -c 32 /dev/urandom | base64
security:
  secret_key: "<base64-32-bytes>"
```

`config.Load` defaults `backend` to `local` and `gdrive.auth` to
`service_account`. `service_account` requires a readable `credentials_file` +
`base_folder_id`. `oauth` fields are all optional at load time — the folder +
token may come from the DB (in-app) or `config.yaml` (bootstrap); the manager
reports whether storage is actually configured.

## Google Cloud one-time setup

Do this once per deployment, regardless of how you configure the folder.

1. Create/choose a Google Cloud project; enable the **Drive API** and (for oauth)
   the **Picker API**.
2. Configure the **OAuth consent screen** — User Type **Internal** if the project
   is under your Workspace org (no verification); otherwise **External** and add
   the user as a *test user*.
3. Create an **OAuth client ID** of type **Web application** and an **API key**
   (the Picker requires it). On the Web client add:
   - Authorized JavaScript origins: the **app's own origin** for the in-app flow
     (e.g. `http://localhost:5173`, plus the deployed origin), and
     `http://localhost:8899` for the CLI tool.
   - Authorized redirect URI: `http://127.0.0.1:8899/callback` (CLI tool only; the
     in-app flow uses `redirect_uri: "postmessage"` and needs no redirect URI).
4. Put the static artifacts (`client_id`, `client_secret`, `api_key`) and a
   `security.secret_key` in `config.yaml`.

## Configuring the folder

### Option A — from the app (recommended)

As an admin, go to **Settings → File storage**:

1. **Connect Google account** — runs OAuth consent in a popup (GIS code client);
   the backend exchanges the code for a refresh token and stores it encrypted.
2. **Choose folder** — opens the Google Picker (My Drive + Shared Drive tabs);
   selecting a folder grants the app `drive.file` access to it, and the backend
   validates + hot-swaps the live store.

Status flips to **● Connected & healthy**. No restart needed.

### Option B — CLI bootstrap (headless)

Same GCP client + user for both tools (so the folder grant applies to the token):

```bash
cd backend
# 1. Grant the base folder via the Picker (prints base_folder_id)
go run ./cmd/gdrive-grant -client-id <web-client-id> -api-key <api-key>
# 2. Mint the refresh token
go run ./cmd/gdrive-auth  -client-id <web-client-id> -client-secret <web-client-secret>
```

Then set `storage.gdrive.base_folder_id` and paste the `oauth:` block into
`config.yaml` and restart. A later in-app save overrides these.

For `service_account` mode instead: create a **Shared Drive** and a base folder,
copy the folder id into `base_folder_id`, and add the service account's email as a
Shared Drive member (**Content Manager**). *Many corporate Workspaces block adding
external / service-account members to Shared Drives — if so, use oauth mode.*

### Notes

- Because the app only ever creates its own PR subfolders/files under the granted
  base folder, `drive.file` is sufficient — it never needs to read pre-existing
  content it didn't create.
- The refresh token is long-lived; reconnect only if it's revoked, if an
  External/Testing consent screen expires it, or if the grant is removed.

## Switching folders / backends

Switching the folder, account, or backend does **not** migrate existing files —
old `stored_path` values point at the previous location and will no longer
resolve. The in-app settings surface this as a warning. Any migration of existing
documents is a separate one-off task.
