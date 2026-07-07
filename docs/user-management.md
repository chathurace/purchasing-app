# User management (admin)

Admins manage who can use the app and what they can do, from a dedicated **Users**
page (`/users`, nav link visible only to admins). The backend enforces admin-only
access on every endpoint; the frontend gating is UX-only.

## Model

Users have always been keyed by their OIDC `sub`, which only exists once a person
logs in. But admins add users **by email**, before any login. Two schema changes
(migration `015_user_management.sql`) bridge that gap:

- **`users.sub` is nullable.** An admin-invited user is a row with an email (and
  optionally pre-assigned roles) but no `sub` yet — a *pending invite*.
- **`users.is_active`** (default true) lets an admin block login without deleting
  the row, preserving the user's history (their purchase requests, reviews, etc.).

Email uniqueness is now **case-insensitive** (`users_email_lower_key` on
`lower(email)`), and emails are stored lowercased, so an invite and the eventual
login resolve to the same row even if the casing differs.

## Login resolution (`repository.ProvisionUserOnLogin`)

On every authenticated request the auth middleware resolves the identity in one
transaction:

1. **Known `sub`** → refresh email/name from the token, return the row.
2. **Pending invite** → a row with `sub IS NULL` whose `lower(email)` matches the
   token email is *claimed* by filling in the real `sub`. This is how "add by
   email" links to the eventual login.
3. **Otherwise** → insert a brand-new self-provisioned user.

After resolution the middleware **refuses login (403) for deactivated users**, then
auto-grants `staff` to any user with no roles (first-timers and freshly-claimed
invites alike). The `bootstrap_admin.email` rule is unchanged.

So: anyone in the IdP can sign in and starts as `staff`; adding them on the Users
page only lets an admin pre-assign roles (or deactivate) before/without a login.

## Roles

The six roles (`staff`, `finance`, `finance_admin`, `admin`, `legal`, `security`)
are unchanged. In the management UI:

- **`staff` is a permanent baseline** — auto-granted, never removable
  (`model.AssignableRoles` excludes it; removing it returns 400).
- All other roles are freely added/removed.
- **Last-admin guard**: you cannot remove the `admin` role from, or deactivate, the
  last active admin (`repository.OtherActiveAdminExists`), and an admin cannot
  deactivate their own account.

## API (all admin-only)

| Method & path | Action |
| --- | --- |
| `GET /api/v1/users` | list users with roles + lifecycle flags (`AdminUser`) |
| `POST /api/v1/users` | invite a user by `{email, name?}` (409 if email exists) |
| `POST /api/v1/users/{id}/roles` | grant a role `{role}` |
| `DELETE /api/v1/users/{id}/roles/{role}` | revoke a role |
| `PUT /api/v1/users/{id}/active` | `{active}` — deactivate / reactivate |

The list view (`AdminUser`) exposes a `pending` flag (`sub IS NULL`, i.e. never
logged in → shown as **Invited**) and `is_active` (→ **Active** / **Deactivated**).
