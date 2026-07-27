import { apiFetch } from "./client";
import type { AdminUser, DirectoryUser, Role, UserSummary } from "../types/api";

export const listUsers = () => apiFetch<AdminUser[]>("/api/v1/users");

// lookupUsers returns the active-user directory (id/email/name) for the approver
// picker. Available to any authenticated user.
export const lookupUsers = () => apiFetch<UserSummary[]>("/api/v1/users/lookup");

// listDirectory returns the org user directory (email/name) from the connected
// identity server via SCIM, for name/email autocomplete. Falls back to the app's
// DB users when SCIM is disabled. Any authenticated user. Pass refresh=true to
// force a fresh SCIM fetch (rate-limited server-side) — used when a typed prefix
// matched nothing cached, so a newly-added user may be missing.
export const listDirectory = (refresh = false) =>
  apiFetch<DirectoryUser[]>(`/api/v1/users/directory${refresh ? "?refresh=1" : ""}`);

// ensureDirectoryUser resolves a directory person to a provisioned app user
// (get-or-create by email), returning their id/email/name so id-based pickers can
// use someone who hasn't logged in yet. procurement_admin/admin only.
export const ensureDirectoryUser = (input: { email: string; name: string }) =>
  apiFetch<UserSummary>("/api/v1/users/ensure", { method: "POST", body: input });

export const createUser = (input: { email: string; name: string }) =>
  apiFetch<AdminUser>("/api/v1/users", { method: "POST", body: input });

// updateUser edits an invited user's email/name. Allowed only while the user is
// still a pending invite (never signed in); the server 409s otherwise.
export const updateUser = (id: number, input: { email: string; name: string }) =>
  apiFetch<AdminUser>(`/api/v1/users/${id}`, { method: "PUT", body: input });

export const addUserRole = (id: number, role: Role) =>
  apiFetch<AdminUser>(`/api/v1/users/${id}/roles`, { method: "POST", body: { role } });

export const removeUserRole = (id: number, role: Role) =>
  apiFetch<AdminUser>(`/api/v1/users/${id}/roles/${role}`, { method: "DELETE" });

export const setUserActive = (id: number, active: boolean) =>
  apiFetch<AdminUser>(`/api/v1/users/${id}/active`, { method: "PUT", body: { active } });
