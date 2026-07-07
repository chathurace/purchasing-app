import { apiFetch } from "./client";
import type { AdminUser, Role, UserSummary } from "../types/api";

export const listUsers = () => apiFetch<AdminUser[]>("/api/v1/users");

// lookupUsers returns the active-user directory (id/email/name) for the approver
// picker. Available to any authenticated user.
export const lookupUsers = () => apiFetch<UserSummary[]>("/api/v1/users/lookup");

export const createUser = (input: { email: string; name: string }) =>
  apiFetch<AdminUser>("/api/v1/users", { method: "POST", body: input });

export const addUserRole = (id: number, role: Role) =>
  apiFetch<AdminUser>(`/api/v1/users/${id}/roles`, { method: "POST", body: { role } });

export const removeUserRole = (id: number, role: Role) =>
  apiFetch<AdminUser>(`/api/v1/users/${id}/roles/${role}`, { method: "DELETE" });

export const setUserActive = (id: number, active: boolean) =>
  apiFetch<AdminUser>(`/api/v1/users/${id}/active`, { method: "PUT", body: { active } });
