# User directory (SCIM autocomplete)

Name/email form fields offer type-to-search **autocomplete suggestions sourced
from the connected identity server** (WSO2 IS / Asgardeo) via its SCIM2 API.
Implements [issue #2496](https://github.com/wso2-enterprise/digiops-cs/issues/2496).

## Why fetch-all-and-cache (not live-per-keystroke)

The org has ~1000 users. The backend fetches the **full** directory from SCIM
into an in-memory cache (TTL, default 10 min) and serves it through the app's own
endpoint; the browser loads it once and filters client-side. This is faster
(instant filtering, no per-keystroke round-trip), resilient (serves stale cache if
a refresh fails), and keeps SCIM load to ~one full fetch per TTL shared across all
users. The browser never sees the SCIM endpoint or the M2M credentials.

> If the directory ever grows past ~10k users, switch to server-side filtered
> search: the `GET /users/directory` endpoint can take a `?q=` and filter the
> cached list server-side without any frontend change.

## Backend

- **Config** — a `scim` block in `config.yaml` (see `config.example.yaml`):
  `enabled`, `base_url`, `token_url`, `client_id`, `client_secret`, `scopes`
  (`internal_user_mgt_list`), `insecure_skip_verify`, `cache_ttl_seconds`,
  `page_size`. **When `enabled` is false (the default), the directory falls back
  to the app's own active DB users**, so dev runs without an M2M app.
- **`internal/directory`** — `scim.go` is the SCIM2 client: it authenticates with
  the OAuth2 **client-credentials** grant (`golang.org/x/oauth2/clientcredentials`,
  auto-refreshing token), pages through `GET {base}/Users` (`startIndex`/`count`,
  requesting only `userName,name.givenName,name.familyName,emails`), and maps each
  resource to `{name, email}` — preferring the primary email, tolerating both the
  bare-string and object email shapes, deriving a display name, and stripping the
  WSO2 user-store prefix (`PRIMARY/…`). `service.go` is the TTL cache with a
  single-flight refresh and the DB fallback.
- **Endpoints** (`handler/users.go`):
  - `GET /api/v1/users/directory` — the full cached `[{name,email}]` list. Any
    authenticated user.
  - `POST /api/v1/users/ensure` `{email,name}` — resolves a directory person to a
    provisioned app user (get-or-create by email via
    `repository.GetOrCreateUserByEmail`, reusing the invite-by-email mechanism —
    the row is claimed on that person's first login) and returns
    `{id,email,name}`. procurement_admin/admin only. Lets **id-based** pickers use
    someone who hasn't logged in yet.

## Frontend

- `api/users.ts` — `listDirectory()` / `ensureDirectoryUser()`; `useDirectory()`
  hook (`hooks/useDirectory.ts`) caches the list per session.
- **`EmailAutocomplete`** — a free-text email field with directory suggestions. It
  does **not** force a selection (a typed email not in the directory is still
  accepted — the point of these fields is people who may not have logged in). On
  picking a suggestion it reports the matched directory user so the caller can also
  fill an associated name field. Used for:
  - **Team lead email** — requisition form + `TeamLeadApprovalCard` edit.
  - **Budget approval chain steps** — add + edit (fills the step's name + email).
  - **Invite user** — the User Management add-user form (fills email + name).
- **`DirectoryUserPicker`** — search-and-add for **id-based** fields. On pick it
  calls `/users/ensure` to resolve/create the app user, then reports the resulting
  `UserSummary` (with an id). Used for:
  - **Business unit approvers** (`BusinessUnitFields`).
  - **Team members** (`TeamsSection`) — adding grants the team's role.

  *Not* used for PR assignee/collaborators: the backend requires those to already
  hold the procurement role, so an arbitrary directory person is invalid there —
  those pickers stay scoped to the procurement team.

## Production prerequisite

Create a **machine-to-machine (client-credentials) application** in WSO2 IS /
Asgardeo holding the **`internal_user_mgt_list`** scope, and populate the `scim.*`
config with its base URL, token endpoint, client id/secret, and scope. Until then
the app runs on the DB-user fallback.
