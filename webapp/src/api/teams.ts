import { apiFetch } from "./client";
import type { Team } from "../types/api";

// Teams API. Reading is open to any authenticated user (the assignee dropdown on
// the approval cards needs team membership); mutations are procurement_admin/admin.

export const listTeams = () => apiFetch<Team[]>("/api/v1/teams");

export const updateTeamEmail = (key: string, teamEmail: string) =>
  apiFetch<Team>(`/api/v1/teams/${key}/email`, {
    method: "PUT",
    body: { team_email: teamEmail },
  });

export const addTeamMember = (key: string, userId: number) =>
  apiFetch<Team>(`/api/v1/teams/${key}/members`, {
    method: "POST",
    body: { user_id: userId },
  });

export const removeTeamMember = (key: string, userId: number) =>
  apiFetch<Team>(`/api/v1/teams/${key}/members/${userId}`, { method: "DELETE" });
