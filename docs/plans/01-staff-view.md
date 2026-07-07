# Plan: Purchasing App — Staff View (Phase 1)

## Context

The company needs an internal app to manage its purchasing function. This is greenfield work in
`purchasing-app/` (currently empty except a `files/` dir and `private/notes`). A sibling project,
`finance-apps/`, is the company's reference architecture and we mirror its conventions:

- **Backend:** Go 1.25 · `go-chi/chi/v5` router · `jackc/pgx/v5` (pgxpool) · `coreos/go-oidc/v3`
  for SSO · `rs/zerolog` logging.
- **Frontend:** React 18 + TypeScript · Vite · React Router v6 · TanStack Query v5 ·
  `oidc-client-ts` · Tailwind CSS (no shadcn — finance-apps uses plain Tailwind).
- **DB:** local PostgreSQL.

Deliberate **deviations from finance-apps** per the request:
1. **Config via `config.yaml`** (finance-apps uses env vars). Use `gopkg.in/yaml.v3`.
2. **Local file storage** (finance-apps uses MinIO). Files live under a configurable root
   (default `purchasing-app/files`), original filenames **preserved**, organized in subfolders.
3. **4 roles maintained inside the app** (not read from the SSO token): `staff`, `finance`,
   `finance_admin`, `admin`. New users are auto-provisioned as `staff` on first login.

This phase builds **only the staff view**: login, list requests, create request, view/edit request.
Finance / finance_admin / admin views and the full procurement workflow (from `docs/Purchasing flow.pdf`)
come in later phases — but the schema and role plumbing are laid so they slot in cleanly.

**Confirmed decisions (from clarifying questions):**
- Line items = **description + quantity** only.
- PR captures a **cost_center** field (no budget amount yet).
- **Minimal state set** for now: `submitted`, `under_review`, `vendor_selected`,
  `contract_prepared`, `completed`, `rejected`, `cancelled`.
- New SSO users default to **`staff`**; an admin grants other roles later (bootstrap admin in config).

---

## Plan archival convention

Plans are preserved **inside the project**, mirroring finance-apps (`finance-apps/CLAUDE.md:17`):
Claude Code overwrites its active plan in `~/.claude/plans/` each session, so the project copy is the
only durable history.

- **First implementation step:** copy this plan to `purchasing-app/docs/plans/01-staff-view.md`.
- Each future approved plan is archived as `purchasing-app/docs/plans/NN-short-name.md` (incrementing NN).
- This convention is recorded in a new `purchasing-app/CLAUDE.md` (created in this phase) so it carries forward.

## Target structure

```
purchasing-app/
├── CLAUDE.md                        # onboarding + plan-archival convention
├── docs/plans/01-staff-view.md      # archived copy of this plan
├── config.yaml                      # gitignored; real config
├── config.example.yaml              # committed template
├── files/                           # default storage root (exists)
├── backend/
│   ├── go.mod                        # module github.com/cs/purchasing-app
│   ├── cmd/server/main.go            # wiring + graceful shutdown
│   ├── migrations/                   # 001..004 .sql
│   └── internal/
│       ├── config/config.go          # YAML loader + struct
│       ├── model/model.go            # PurchaseRequest, Item, Link, Document, User, status consts
│       ├── repository/repository.go  # pgx queries
│       ├── repository/json.go        # marshal/unmarshal helpers (copied)
│       ├── middleware/auth.go        # OIDC verify + upsert + default-staff + roles on ctx
│       ├── middleware/cors.go        # copied
│       ├── storage/local.go          # save/open/delete under files root, preserve filename
│       └── handler/
│           ├── router.go             # chi routes
│           ├── respond.go            # writeJSON/writeError/parseID/decodeJSON (copied)
│           ├── users.go              # Me
│           └── purchase_requests.go  # list/create/get/update + documents up/down/delete
└── frontend/
    ├── package.json · vite.config.ts · tsconfig.json · tailwind.config.js · index.css
    ├── .env.example                  # VITE_OIDC_* + VITE_API_BASE_URL
    └── src/
        ├── main.tsx · App.tsx
        ├── auth/{userManager,AuthContext,RequireAuth}.tsx
        ├── api/{client,me,purchaseRequests}.ts
        ├── hooks/{useMe,usePurchaseRequests}.ts
        ├── types/api.ts
        ├── components/{Layout,StatusBadge,FileList}.tsx
        └── pages/{LoginPage,CallbackPage,PurchaseRequestListPage,NewPurchaseRequestPage,PurchaseRequestDetailPage}.tsx
```

Reuse, near-verbatim, from `finance-apps/backend`: `handler/respond.go`, `middleware/cors.go`,
`repository/json.go`, the pgxpool/`dbConn` abstraction and `scanUser` NULL-handling pattern
(`repository/repository.go:14-40, 52-71, 168-182`), and `cmd/server/main.go` wiring skeleton
(`cmd/server/main.go:28-87`). On the frontend, mirror `auth/`, `api/client.ts` (`apiFetch<T>`),
`main.tsx`, and `components/Layout.tsx` wholesale.

---

## Database schema

Migrations are numbered `NNN_description.sql`, applied with
`psql -h localhost -d purchasing -f <file>` (finance-apps convention).

**`001_users_roles.sql`** — identical shape to finance-apps `001`, different role seed:
```sql
CREATE TABLE users (
    id         BIGSERIAL    PRIMARY KEY,
    sub        TEXT         NOT NULL UNIQUE,   -- OIDC subject
    email      TEXT         NOT NULL UNIQUE,
    name       TEXT         NOT NULL,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE TABLE roles (
    id   BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE
);
CREATE TABLE user_roles (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id BIGINT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, role_id)
);
INSERT INTO roles (name) VALUES ('staff'), ('finance'), ('finance_admin'), ('admin');
```

**`002_purchase_requests.sql`**
```sql
CREATE TABLE purchase_requests (
    id           BIGSERIAL   PRIMARY KEY,           -- displayed as PR-{id:06d}
    title        TEXT        NOT NULL DEFAULT '',    -- short summary for the list view
    requester_id BIGINT      NOT NULL REFERENCES users(id),
    cost_center  TEXT        NOT NULL DEFAULT '',
    comments     TEXT        NOT NULL DEFAULT '',
    status       TEXT        NOT NULL DEFAULT 'submitted',
        -- submitted | under_review | vendor_selected | contract_prepared
        -- | completed | rejected | cancelled
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_pr_requester ON purchase_requests(requester_id);
CREATE INDEX idx_pr_status    ON purchase_requests(status);
```

**`003_pr_items_links.sql`**
```sql
CREATE TABLE pr_items (
    id                  BIGSERIAL PRIMARY KEY,
    purchase_request_id BIGINT NOT NULL REFERENCES purchase_requests(id) ON DELETE CASCADE,
    description         TEXT   NOT NULL,
    quantity            NUMERIC(14,3) NOT NULL DEFAULT 1,
    position            INT    NOT NULL DEFAULT 0
);
CREATE INDEX idx_pr_items_pr ON pr_items(purchase_request_id);

CREATE TABLE pr_links (
    id                  BIGSERIAL PRIMARY KEY,
    purchase_request_id BIGINT NOT NULL REFERENCES purchase_requests(id) ON DELETE CASCADE,
    url                 TEXT   NOT NULL,
    label               TEXT   NOT NULL DEFAULT '',
    position            INT    NOT NULL DEFAULT 0
);
CREATE INDEX idx_pr_links_pr ON pr_links(purchase_request_id);
```

**`004_documents.sql`** — mirrors finance-apps `006`, but `stored_path` (local relative path)
replaces MinIO `object_key`:
```sql
CREATE TABLE documents (
    id                  BIGSERIAL   PRIMARY KEY,
    purchase_request_id BIGINT      NOT NULL REFERENCES purchase_requests(id) ON DELETE CASCADE,
    filename            TEXT        NOT NULL,        -- original name, preserved
    stored_path         TEXT        NOT NULL,        -- relative to files root
    content_type        TEXT        NOT NULL DEFAULT 'application/octet-stream',
    size_bytes          BIGINT      NOT NULL DEFAULT 0,
    uploaded_by         BIGINT      REFERENCES users(id),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_documents_pr ON documents(purchase_request_id);
```

*(A `pr_status_history` audit table is deferred to the phase that introduces state transitions; the
current status field is sufficient for the staff view.)*

---

## File storage (local)

`internal/storage/local.go` — root from config (`storage.files_root`, default `purchasing-app/files`).
Layout preserves the exact original filename inside a per-document UUID folder (avoids collisions
without renaming), echoing finance-apps' `{id}/{uuid}/{filename}` MinIO key:

```
<files_root>/purchase-requests/<prID>/<docUUID>/<originalFilename>
```

- `Save(prID int64, originalName string, r io.Reader) (storedPath string, size int64, err error)` —
  generates `docUUID` (`google/uuid`), `os.MkdirAll` the dir, streams the file, returns the path
  **relative to root** for the DB.
- `Open(storedPath string) (io.ReadCloser, error)` and `Delete(storedPath string) error`.
- Path safety: reject any `storedPath` that escapes root (`filepath.Clean` + prefix check) before open/delete.
- Allowed uploads: **pdf, docx** only — validate by extension + content type, reject others (400).

---

## Backend behavior

**Config (`config.yaml`)** loaded via `gopkg.in/yaml.v3`:
```yaml
server:
  port: 8081                       # finance-apps uses 8080; avoid clash
database:
  url: "postgres://chathura@localhost:5432/purchasing?sslmode=disable"
oidc:
  issuer: "https://localhost:9443/oauth2/token"
  discovery_url: "https://localhost:9443/oauth2/.well-known/openid-configuration"  # WSO2 IS
  client_id: "<wso2-client-id>"
  insecure_skip_verify: true       # dev only (self-signed WSO2 cert)
storage:
  files_root: "purchasing-app/files"
bootstrap_admin:
  email: "chathura@wso2.com"
cors:
  allowed_origins: ["http://localhost:5173"]
```
`config.Load(path)` reads the file (path from `-config` flag or `PURCHASING_CONFIG` env, default
`./config.yaml`), unmarshals, applies defaults, validates required fields. `config.example.yaml`
committed; `config.yaml` gitignored.

**Auth (`middleware/auth.go`)** — adapts finance-apps `auth.go`:
- `go-oidc` provider built from `discovery_url` (override) so WSO2's discovery endpoint works even
  when the issuer claim differs; honor `insecure_skip_verify` for the dev self-signed cert.
- Verify JWT → extract `{sub, email, name/preferred_username}` → `repo.UpsertUser` (ON CONFLICT sub).
- **Auto-provision:** on upsert of a brand-new user, grant the `staff` role. If the email matches
  `bootstrap_admin.email`, idempotently grant `admin` on every login.
- Load roles from `user_roles` and place user + roles on request context (`CtxUser`, `CtxRoles`);
  reuse `UserFromCtx` / `RolesFromCtx` / `HasRole` helpers.

**Routes (`/api/v1`, all behind auth):**
```
GET    /me                                          -> {id, sub, email, name, roles[]}
GET    /purchase-requests                           -> staff: own PRs; finance*/admin: all (forward-compat)
POST   /purchase-requests                           -> create (status=submitted); body: title, cost_center,
                                                        comments, items[], links[]
GET    /purchase-requests/{id}                       -> full PR + items + links + documents
PUT    /purchase-requests/{id}                        -> update title/cost_center/comments/items/links
                                                        (owner only; only if status in submitted|under_review)
POST   /purchase-requests/{id}/documents              -> multipart upload (pdf/docx); editable-state + owner only
GET    /purchase-requests/{id}/documents/{docID}/download -> streams file, Content-Disposition = original name
DELETE /purchase-requests/{id}/documents/{docID}      -> remove file + row; editable-state + owner only
```
- **Editability rule** (single helper): a staff user may edit/add/remove docs only when they own the
  PR and `status ∈ {submitted, under_review}`; otherwise 403. Admin bypasses (forward-compat).
- Items/links updates on PUT = replace-all inside a transaction (delete + re-insert, like
  finance-apps `SetUserRoles` at `repository.go:120-139`).
- Upload streams directly to `storage.Save` (no presigned URLs — local disk); download streams via
  `storage.Open`.

**Server wiring (`cmd/server/main.go`)**: load config → pgxpool connect + ping → `repository.New` →
`storage.NewLocalStore(root)` → `middleware.NewAuth(...)` → `handler.NewRouter(Deps{...})` →
`http.Server` with timeouts + graceful shutdown on SIGINT/SIGTERM. zerolog to stdout + `logs/server.log`.

---

## Frontend behavior (staff view)

Mirror finance-apps frontend. **Routes (`App.tsx`):** `/login`, `/callback` (public);
everything else under `RequireAuth > Layout`:
- `/` or `/requests` → **PurchaseRequestListPage** — `usePurchaseRequests()` (TanStack Query,
  `refetchInterval: 5000`); table of PR-{id}, title, status (`StatusBadge`), created date; "New request" button.
- `/requests/new` → **NewPurchaseRequestPage** — form: title, cost_center, comments, dynamic
  **items** rows (description + quantity, add/remove), dynamic **links** rows (url + label), file
  upload (pdf/docx, multiple). On submit: `POST /purchase-requests`, then upload each file, then
  navigate to detail.
- `/requests/:id` → **PurchaseRequestDetailPage** — shows all details + current status. If status is
  `submitted` or `under_review` **and** the viewer is the owner, render edit mode: editable fields/items/links
  (PUT on save), add/remove documents (upload + delete endpoints). Otherwise read-only.

**Auth:** copy `auth/userManager.ts` (UserManager from `VITE_OIDC_*`), `AuthContext`, `RequireAuth`,
`CallbackPage`. `api/client.ts` `apiFetch<T>` injects `Authorization: Bearer <token>`. `useMe()`
queries `/me`; `Layout` shows the user's email + logout and (forward-compat) renders nav by role —
for now just the staff "Requests" link. **File upload uses multipart `FormData`** to the backend
(not presigned URLs), so `apiFetch` must support a `FormData` body (skip JSON content-type when body
is `FormData`).

`.env.example`: `VITE_API_BASE_URL=http://localhost:8081`, `VITE_OIDC_AUTHORITY`,
`VITE_OIDC_CLIENT_ID`, `VITE_OIDC_REDIRECT_URI=http://localhost:5173/callback`,
`VITE_OIDC_SCOPE=openid profile email` — values mirror the backend `oidc` config.

---

## Licensing note

All proposed libraries are MIT/Apache-2.0/BSD compatible: chi (MIT), pgx (MIT), go-oidc (Apache-2.0),
zerolog (MIT), yaml.v3 (MIT/Apache), google/uuid (BSD-3); React/Vite/React Router/TanStack
Query/oidc-client-ts/Tailwind all MIT. No GPL/AGPL dependencies introduced.

---

## Verification

1. **DB:** `createdb purchasing`; apply migrations
   `for f in backend/migrations/*.sql; do psql -h localhost -d purchasing -f "$f"; done`;
   confirm `SELECT name FROM roles;` → staff/finance/finance_admin/admin.
2. **Backend:** `cd backend && go build ./...` (clean), then `go run ./cmd/server` with a real
   `config.yaml`; confirm it listens on :8081 and logs OIDC discovery success.
3. **Frontend:** `cd frontend && npm install && npm run dev`; open http://localhost:5173.
4. **Login:** click "Sign in with SSO" → WSO2 IS login → redirected back; `GET /me` returns the user
   with `roles: ["staff"]` (auto-provisioned); psql `SELECT * FROM user_roles` confirms the grant.
5. **Create:** new request with 2 items, a link, one `.pdf` and one `.docx`; verify rows in
   `purchase_requests`/`pr_items`/`pr_links`/`documents` and files on disk at
   `files/purchase-requests/<id>/<uuid>/<originalName>` (names preserved).
6. **Reject bad upload:** attempt a `.png` upload → 400.
7. **List:** the new PR appears with status `submitted`.
8. **Edit (allowed):** while `submitted`, change comments + add/remove a document → persists; download
   a document → original filename in the browser.
9. **Edit (blocked):** `UPDATE purchase_requests SET status='vendor_selected'` in psql; reload detail →
   read-only, and a direct `PUT`/document `DELETE` returns 403.
10. **Ownership:** a second SSO user cannot see or edit the first user's PR (list excludes it; direct
    GET/PUT → 403/404).
