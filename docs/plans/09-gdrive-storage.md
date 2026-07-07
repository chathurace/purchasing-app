# Google Drive file-storage backend

## Context

Every document in the PR flow (quotations, contracts, GRNs, invoices, RFI/comment attachments,
signed PDFs) is currently written to the local filesystem under `storage.files_root`. All file I/O
is already centralized in `backend/internal/storage/local.go` (`LocalStore` with `Save`/`Open`/`Delete`),
and every handler touches files only through the three helpers in
`backend/internal/handler/documents.go`. There are **no scattered filesystem calls**.

We want a second storage option: **Google Drive**, accessed *as the app* via a preconfigured
service account. A single configurable base folder holds one subfolder per PR (named by the PR id),
and all of that PR's files live inside it.

Decisions (confirmed with the user):
- **Auth**: service-account JSON key + a Google **Shared Drive**. The base folder lives in a Shared
  Drive the service account is a member of. This sidesteps the well-known trap that plain service
  accounts have no personal Drive storage quota (uploads to My Drive fail); Shared Drive storage is
  owned by the org.
- **One backend chosen at startup** (`local` OR `gdrive`) via config. No per-file mixing, no
  automatic migration of existing files when switching.

The design leans on the fact that `documents.stored_path` (TEXT) is an **opaque token** that flows
Save → DB → Open/Delete and is never interpreted by callers — so each backend defines its own format.

## Approach

### 1. Extract a `Store` interface — `backend/internal/storage/store.go` (new)

```go
package storage

import "io"

// Store abstracts document blob storage. Implemented by LocalStore and GDriveStore.
// relPath is an opaque token persisted verbatim in documents.stored_path and handed
// back to Open/Delete; its format is implementation-defined and callers must not parse it.
type Store interface {
	Save(prID int64, originalName string, r io.Reader) (relPath string, size int64, err error)
	Open(relPath string) (io.ReadCloser, error)
	Delete(relPath string) error
}

var _ Store = (*LocalStore)(nil)
```

- `LocalStore` already satisfies this exactly — **no changes to `local.go`**.
- **`Root()` stays off the interface** (filesystem-specific, used only by the startup log line at
  `cmd/server/main.go:52`). It remains a `*LocalStore`-only method; the local startup-log branch calls
  it directly.

### 2. Retype handlers/Deps to the interface (mechanical)

Change the concrete `*storage.LocalStore` to `storage.Store` in these places (no logic changes —
handlers only ever call `Save`/`Open`/`Delete`):
- `handler/router.go` — `Deps.Storage`
- `handler/documents.go` — the `store` param of `saveUploadedDoc`, `downloadOwnedDoc`, `deleteOwnedDoc`
- `handler/purchase_requests.go`, `quotations.go`, `contracts.go`, `grns.go`, `invoices.go` — the
  `Storage` struct field on each handler (`recommendations.go` uses `h.Storage` off
  `PurchaseRequestsHandler`, so it's covered).

### 3. `GDriveStore` — `backend/internal/storage/gdrive.go` (new)

Drive layout: `<base_folder_id>/<prID>/<originalFilename>`.

- **Construction** — `drive.NewService(ctx, option.WithCredentialsFile(credsFile), option.WithScopes(drive.DriveScope))`.
  Use **full `drive` scope** (not `drive.file`): `drive.file` can't see a base folder a human created
  in the Drive UI, which would break the startup `Files.Get` and folder listing.
  At startup, `Files.Get(baseFolderID).SupportsAllDrives(true).Fields("id","driveId","mimeType")` to
  (a) fail fast with a "check sharing to the service account" message if the SA can't see it — the #1
  setup mistake — and (b) capture `driveId` for scoping List queries.
- **Save** — find-or-create the per-PR folder, then `Files.Create(&drive.File{Name, Parents:[folderID]}).Media(reader, googleapi.ContentType("application/octet-stream")).SupportsAllDrives(true).Fields("id","size")`.
  Returns `"gdrive:" + fileID` as the opaque token, and the created file's `Size` (with a
  byte-counting reader wrapper as a fallback).
- **Open** — `Files.Get(id).SupportsAllDrives(true).Download()` returns an `*http.Response`; return
  `resp.Body` wrapped so `Close()` also cancels the request context (the handler streams after this
  returns).
- **Delete** — `Files.Delete(id).SupportsAllDrives(true).Do()`; map a Drive **404 → nil** to match
  `LocalStore`'s `os.IsNotExist` tolerance (the save-rollback and delete paths must be idempotent).
- **Shared-Drive params are mandatory**: every call sets `SupportsAllDrives(true)`; List calls add
  `IncludeItemsFromAllDrives(true)` and, when `driveId != ""`, `Corpora("drive").DriveId(driveId)`.
- **Find-or-create folder** — guarded by a single `sync.Mutex` + an in-process
  `folderCache map[int64]string` (prID → folder ID). This prevents the concurrent-upload race that
  would create duplicate `<prID>` folders and skips a List round-trip on repeat uploads. If duplicates
  ever exist, pick `files[0]` (graceful degradation). *Caveat to document: the guarantee is
  per-process; fine for the single-binary deployment.*
- **Filenames**: flat inside the PR folder — **no per-doc UUID subfolder**. Drive keys by file ID, so
  duplicate display names are legal and unambiguous. Reuse the existing package-level `sanitizeName`
  from `local.go`.
- **Context**: keep the interface ctx-free; `GDriveStore` uses `context.WithTimeout(context.Background(), …)`
  internally — 60s for folder/delete ops, 10 min for upload/download (download's context is tied to
  the body's `Close()`). Matches `LocalStore`'s ctx-free `os` calls; richer cancellation is a
  mechanical follow-up if ever needed.
- `var _ Store = (*GDriveStore)(nil)`.

### 4. Config — `backend/internal/config/config.go` + `config.example.yaml`

Extend the `Storage` struct:

```go
Storage struct {
	Backend   string `yaml:"backend"` // "local" (default) or "gdrive"
	FilesRoot string `yaml:"files_root"`
	GDrive    struct {
		CredentialsFile string `yaml:"credentials_file"`
		BaseFolderID    string `yaml:"base_folder_id"`
	} `yaml:"gdrive"`
} `yaml:"storage"`
```

In `Load`: default `Backend` to `"local"`; then `switch` — for `gdrive` require `credentials_file`
and `base_folder_id` and `os.Stat` the creds file; unknown backend → error. Add the `gdrive` block
(commented, with `backend: "local"`) to `config.example.yaml`. The creds JSON file follows the same
gitignore pattern as `config.yaml` (note in the example/comment; **do not** commit the key).

### 5. Wiring — `backend/cmd/server/main.go`

Replace the fixed `NewLocalStore` construction (lines 48–52) with a branch on `cfg.Storage.Backend`
producing a `storage.Store`:
- `gdrive` → `storage.NewGDriveStore(ctx, creds, baseFolderID)`, log `base_folder_id`.
- default `local` → `storage.NewLocalStore(...)`, log `root` via the `*LocalStore.Root()` method.

Pass `store` into `handler.Deps{Storage: store, …}` unchanged.

### 6. Dependencies — `backend/go.mod`

Add direct require `google.golang.org/api` (latest) and run `go mod tidy`. This is a **heavy** tree
(pulls `cloud.google.com/go/auth`, `gax-go`, grpc/protobuf, opentelemetry) — unavoidable with the
official client; accepted over hand-rolling REST. `golang.org/x/oauth2`, `x/sync`, `x/sys`, `x/text`
are already indirect deps (via go-oidc), so those don't re-download.

## Implementation sequencing

1. `storage/store.go` — interface + `LocalStore` assertion (compiles against existing code).
2. Retype the Deps field, 3 `documents.go` helpers, and 5 handler struct fields to `storage.Store`.
3. `config.go` struct + validation; update `config.example.yaml`.
4. `go.mod`: add `google.golang.org/api`; `go mod tidy`.
5. `storage/gdrive.go` — implement `GDriveStore` + assertion.
6. `main.go` — branch on backend.
7. Docs: archive this plan to `docs/plans/09-gdrive-storage.md`; after implementation add a short
   `docs/file-storage.md` as-built (backends, config, Shared-Drive setup steps, stored_path formats).

## Verification

**Build/regression (local backend unchanged):**
- `cd backend && go build ./... && go vet ./...`.
- Run with the existing `config.yaml` (backend defaults to `local`); confirm the startup log still
  prints `local file storage initialized root=…`.
- In the UI, upload → download → delete a document on a PR / quotation / contract; confirm bytes
  round-trip and the file lands under `files_root/purchase-requests/<id>/…` as before.

**Google Drive backend (needs a real SA + Shared Drive):**
- One-time setup: create a service account + JSON key; create a Shared Drive and a base folder in it;
  add the SA as a **Content Manager** member of the Shared Drive; enable the Drive API on the project.
- Point `config.yaml` at `backend: "gdrive"`, `credentials_file`, `base_folder_id`. Start the server;
  confirm `google drive file storage initialized base_folder_id=…` and **no** startup error (a
  sharing mistake fails fast here).
- Upload a doc to a PR → confirm a `<prID>` subfolder appears under the base folder in the Drive UI
  with the file inside, and that `documents.stored_path` is `gdrive:<fileId>` in Postgres.
- Download it → bytes match the original; correct filename in `Content-Disposition`.
- Upload a second file with the same name → both coexist (distinct IDs), both downloadable.
- Delete → file disappears from Drive; deleting again (or the save-rollback path) does not error.
- Optional: the MCP Google Drive tools (`search_files`, `get_file_metadata`,
  `download_file_content`) can independently confirm the folder/file layout during testing.

## Out of scope / notes
- No automatic migration of existing local files when switching backends (one-off, if ever needed).
- Single-process folder-create guarantee (documented caveat); fine for the single-binary deployment.
