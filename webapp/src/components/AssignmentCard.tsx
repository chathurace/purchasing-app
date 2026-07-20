import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError } from "../api/client";
import {
  addPRCollaborator,
  removePRCollaborator,
  setPRAssignee,
} from "../api/purchaseRequests";
import { listTeams } from "../api/teams";
import type { Me, PurchaseRequest, UserSummary } from "../types/api";
import { ApproverPicker } from "./ApproverPicker";

function userLabel(u: UserSummary): string {
  return u.name ? `${u.name} (${u.email})` : u.email || `#${u.id}`;
}

// AssignmentCard renders the PR-assignment gate that sits between team-lead approval
// and procurement work. A procurement user is the PR's assignee; zero or more
// collaborators share full work access. Any procurement user can claim an unassigned
// PR (or hand back their own); a procurement_admin/admin can assign/reassign anyone
// via the dropdown and manage collaborators. Only shown once the team lead has
// approved (before then procurement can't see the PR at all).
export function AssignmentCard({ pr, me }: { pr: PurchaseRequest; me: Me | undefined }) {
  const qc = useQueryClient();
  const [error, setError] = useState<string | null>(null);

  const isProcurementAdmin =
    !!me && (me.roles.includes("procurement_admin") || me.roles.includes("admin"));
  const canAssign = !!pr.my_can_assign;
  const canManage = !!pr.my_can_manage_collaborators;

  // Only relevant once the team lead has approved; hidden otherwise. Requesters and
  // other viewers with no controls still see who (if anyone) is handling the PR.
  const visible = pr.team_lead_status === "approved" && (canAssign || canManage || !!pr.assignee || !!me);

  const { data: teams } = useQuery({
    queryKey: ["teams"],
    queryFn: listTeams,
    enabled: visible && (canAssign || canManage),
  });
  // Assignable pool = the Procurement team (plain `procurement` role holders) plus
  // the current user when they're a procurement_admin/admin but not already a team
  // member — so a procurement_admin can pick (assign to) themselves.
  const members: UserSummary[] = useMemo(() => {
    const teamMembers = teams?.find((t) => t.key === "procurement")?.members ?? [];
    if (me && isProcurementAdmin && !teamMembers.some((m) => m.id === me.id)) {
      return [{ id: me.id, email: me.email, name: me.name }, ...teamMembers];
    }
    return teamMembers;
  }, [teams, me, isProcurementAdmin]);

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ["purchase-requests", pr.id] });
    qc.invalidateQueries({ queryKey: ["purchase-requests"] });
  };

  const assignMutation = useMutation({
    mutationFn: (assigneeId: number | null) => setPRAssignee(pr.id, assigneeId),
    onSuccess: () => {
      invalidate();
      setError(null);
    },
    onError: (e: unknown) => setError(e instanceof ApiError ? e.message : "Failed to update assignee"),
  });

  const addCollab = useMutation({
    mutationFn: (userId: number) => addPRCollaborator(pr.id, userId),
    onSuccess: () => {
      invalidate();
      setError(null);
    },
    onError: (e: unknown) => setError(e instanceof ApiError ? e.message : "Failed to add collaborator"),
  });

  const removeCollab = useMutation({
    mutationFn: (userId: number) => removePRCollaborator(pr.id, userId),
    onSuccess: () => {
      invalidate();
      setError(null);
    },
    onError: (e: unknown) => setError(e instanceof ApiError ? e.message : "Failed to remove collaborator"),
  });

  if (!visible) return null;

  const busy = assignMutation.isPending;
  const assigned = !!pr.assignee_id;
  const collaborators = pr.collaborators ?? [];
  // Assignee + existing collaborators are hidden from the collaborator add-list.
  const collabSelectedIds = [
    ...(pr.assignee_id ? [pr.assignee_id] : []),
    ...collaborators.map((c) => c.id),
  ];

  return (
    <div className="mt-6 rounded border bg-white p-6">
      <div className="mb-3 flex items-center justify-between">
        <h2 className="font-medium text-gray-900">Assignment</h2>
        <span
          className={`rounded px-2 py-0.5 text-xs font-medium ${
            assigned ? "bg-green-100 text-green-800" : "bg-amber-100 text-amber-800"
          }`}
        >
          {assigned ? "Assigned" : "Unassigned"}
        </span>
      </div>

      {/* Assignee */}
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-sm text-gray-600">Assignee:</span>
        <span className="text-sm font-medium text-gray-900">
          {pr.assignee ? userLabel(pr.assignee) : "—"}
        </span>
      </div>

      {canAssign && (
        <div className="mt-2 flex flex-wrap items-center gap-2">
          {isProcurementAdmin ? (
            <select
              className="min-w-[16rem] rounded border px-2 py-1 text-sm focus:border-indigo-500 focus:outline-none disabled:opacity-50"
              value={pr.assignee_id ?? ""}
              disabled={busy}
              onChange={(e) =>
                assignMutation.mutate(e.target.value === "" ? null : Number(e.target.value))
              }
            >
              <option value="">Unassigned</option>
              {/* Keep a stale assignee visible even if they're not in the member list. */}
              {pr.assignee && !members.some((m) => m.id === pr.assignee_id) && (
                <option value={pr.assignee_id ?? ""}>{userLabel(pr.assignee)}</option>
              )}
              {members.map((m) => (
                <option key={m.id} value={m.id}>
                  {userLabel(m)}
                </option>
              ))}
            </select>
          ) : !assigned ? (
            <button
              type="button"
              className="rounded bg-indigo-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-indigo-700 disabled:opacity-50"
              onClick={() => me && assignMutation.mutate(me.id)}
              disabled={busy}
            >
              {busy ? "Assigning…" : "Assign to me"}
            </button>
          ) : (
            me &&
            pr.assignee_id === me.id && (
              <button
                type="button"
                className="rounded border px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-50 disabled:opacity-50"
                onClick={() => assignMutation.mutate(null)}
                disabled={busy}
              >
                {busy ? "Working…" : "Unassign me"}
              </button>
            )
          )}
        </div>
      )}

      {!assigned && canAssign && (
        <p className="mt-2 text-sm text-gray-400">
          This request must be assigned to a procurement user before quotations or a
          recommendation can be added.
        </p>
      )}

      {/* Collaborators */}
      <div className="mt-4">
        <p className="mb-1 text-sm text-gray-600">Collaborators</p>
        {canManage ? (
          <ApproverPicker
            candidates={members}
            selectedIds={collabSelectedIds}
            selectedUsers={collaborators}
            onAdd={(u) => addCollab.mutate(u.id)}
            onRemove={(id) => removeCollab.mutate(id)}
            disabled={addCollab.isPending || removeCollab.isPending}
          />
        ) : collaborators.length > 0 ? (
          <ul className="flex flex-wrap gap-2">
            {collaborators.map((c) => (
              <li
                key={c.id}
                className="rounded-full bg-indigo-50 px-2.5 py-1 text-xs text-indigo-700"
              >
                {userLabel(c)}
              </li>
            ))}
          </ul>
        ) : (
          <p className="text-sm text-gray-400">No collaborators.</p>
        )}
      </div>

      {error && <p className="mt-2 text-sm text-red-600">{error}</p>}
    </div>
  );
}
