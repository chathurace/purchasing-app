import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError } from "../api/client";
import { listTeams, updateTeamEmail, addTeamMember, removeTeamMember } from "../api/teams";
import { lookupUsers } from "../api/users";
import { ROLE_LABELS } from "../types/api";
import type { Team, UserSummary } from "../types/api";

const inputCls = "rounded border px-3 py-2 text-sm focus:border-indigo-500 focus:outline-none";

// TeamsSection manages the Legal / Security / Procurement teams from the Settings
// page: each team's shared email and its members (adding/removing a member
// grants/revokes the team's member role). The member role itself is fixed —
// shown, not editable — because the approval-card actor logic is keyed on it.
export function TeamsSection() {
  const qc = useQueryClient();
  const { data: teams, isLoading, error } = useQuery({ queryKey: ["teams"], queryFn: listTeams });
  const { data: users } = useQuery({ queryKey: ["users_lookup"], queryFn: lookupUsers });
  const [actionError, setActionError] = useState<string | null>(null);

  const invalidate = () => qc.invalidateQueries({ queryKey: ["teams"] });
  const onError = (e: unknown) =>
    setActionError(e instanceof ApiError ? e.message : "Action failed");

  const setEmail = useMutation({
    mutationFn: ({ key, email }: { key: string; email: string }) => updateTeamEmail(key, email),
    onSuccess: () => {
      setActionError(null);
      invalidate();
    },
    onError,
  });
  const addMember = useMutation({
    mutationFn: ({ key, userId }: { key: string; userId: number }) => addTeamMember(key, userId),
    onSuccess: () => {
      setActionError(null);
      invalidate();
    },
    onError,
  });
  const removeMember = useMutation({
    mutationFn: ({ key, userId }: { key: string; userId: number }) => removeTeamMember(key, userId),
    onSuccess: () => {
      setActionError(null);
      invalidate();
    },
    onError,
  });

  return (
    <section>
      <h2 className="mb-1 text-lg font-semibold text-gray-900">Teams</h2>
      <p className="mb-4 text-sm text-gray-500">
        Members of a team hold its role — adding or removing a member grants or revokes that role.
        The team email is CC'd on the team's approval notifications.
      </p>

      {actionError && (
        <div className="mb-4 rounded border border-red-200 bg-red-50 px-4 py-2 text-sm text-red-700">
          {actionError}
        </div>
      )}
      {isLoading && <p className="text-gray-500">Loading…</p>}
      {error && <p className="text-red-600">Failed to load teams.</p>}

      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        {(teams ?? []).map((team) => (
          <TeamCard
            key={team.key}
            team={team}
            users={users ?? []}
            busy={setEmail.isPending || addMember.isPending || removeMember.isPending}
            onSaveEmail={(email) => setEmail.mutate({ key: team.key, email })}
            onAddMember={(userId) => addMember.mutate({ key: team.key, userId })}
            onRemoveMember={(userId) => removeMember.mutate({ key: team.key, userId })}
          />
        ))}
      </div>
    </section>
  );
}

function TeamCard({
  team,
  users,
  busy,
  onSaveEmail,
  onAddMember,
  onRemoveMember,
}: {
  team: Team;
  users: UserSummary[];
  busy: boolean;
  onSaveEmail: (email: string) => void;
  onAddMember: (userId: number) => void;
  onRemoveMember: (userId: number) => void;
}) {
  const [email, setEmail] = useState(team.team_email);
  const emailDirty = email.trim() !== team.team_email;

  // Tolerate an older API response that predates admin_members.
  const adminMembers = team.admin_members ?? [];
  const adminIds = useMemo(() => new Set(adminMembers.map((m) => m.id)), [adminMembers]);
  // Base members are shown as removable rows; anyone who is also an admin is lifted
  // out into the badged, non-removable admin group (admins are managed on Users).
  const baseMembers = useMemo(
    () => team.members.filter((m) => !adminIds.has(m.id)),
    [team.members, adminIds],
  );
  const memberIds = useMemo(
    () => new Set([...team.members, ...adminMembers].map((m) => m.id)),
    [team.members, adminMembers],
  );
  const addable = useMemo(() => users.filter((u) => !memberIds.has(u.id)), [users, memberIds]);
  const totalMembers = baseMembers.length + adminMembers.length;

  return (
    <div className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm">
      <div className="flex items-center justify-between">
        <h3 className="text-base font-semibold text-gray-900">{team.name}</h3>
        <span className="badge bg-slate-100 text-slate-600 ring-slate-500/20">
          {ROLE_LABELS[team.member_role]}
        </span>
      </div>

      {/* Team email */}
      <label className="mt-4 block text-xs font-semibold uppercase tracking-wide text-slate-400">
        Team email
      </label>
      <div className="mt-1 flex gap-2">
        <input
          type="email"
          className={`${inputCls} flex-1`}
          placeholder="team@example.com"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
        />
        <button
          type="button"
          disabled={busy || !emailDirty}
          onClick={() => onSaveEmail(email.trim())}
          className="rounded bg-indigo-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-indigo-700 disabled:opacity-50"
        >
          Save
        </button>
      </div>

      {/* Members */}
      <p className="mt-4 text-xs font-semibold uppercase tracking-wide text-slate-400">
        Members ({totalMembers})
      </p>
      {totalMembers === 0 ? (
        <p className="mt-1 text-sm text-gray-400">No members yet.</p>
      ) : (
        <ul className="mt-1 space-y-1">
          {baseMembers.map((m) => (
            <li
              key={m.id}
              className="flex items-center justify-between rounded border border-slate-100 bg-slate-50/60 px-2.5 py-1.5 text-sm"
            >
              <span className="min-w-0 truncate text-gray-700" title={m.email}>
                {m.email || m.name || `#${m.id}`}
              </span>
              <button
                type="button"
                disabled={busy}
                onClick={() => onRemoveMember(m.id)}
                className="ml-2 shrink-0 text-xs text-red-600 hover:underline disabled:opacity-50"
              >
                Remove
              </button>
            </li>
          ))}
          {adminMembers.map((m) => (
            <li
              key={m.id}
              className="flex items-center justify-between gap-2 rounded border border-slate-100 bg-slate-50/60 px-2.5 py-1.5 text-sm"
            >
              <span className="min-w-0 truncate text-gray-700" title={m.email}>
                {m.email || m.name || `#${m.id}`}
              </span>
              <span
                className="badge shrink-0 bg-amber-100 text-amber-700 ring-amber-500/20"
                title="Admin — manage this role on the Users page"
              >
                admin
              </span>
            </li>
          ))}
        </ul>
      )}
      {adminMembers.length > 0 && (
        <p className="mt-1 text-xs text-slate-400">Admins are managed on the Users page.</p>
      )}

      {/* Add member */}
      <select
        className={`${inputCls} mt-2 w-full border-dashed text-gray-500`}
        value=""
        disabled={busy || addable.length === 0}
        onChange={(e) => {
          const id = Number(e.target.value);
          if (id) onAddMember(id);
          e.currentTarget.value = "";
        }}
      >
        <option value="">{addable.length === 0 ? "All users are members" : "+ Add member"}</option>
        {addable.map((u) => (
          <option key={u.id} value={u.id}>
            {u.email || u.name || `#${u.id}`}
          </option>
        ))}
      </select>
    </div>
  );
}
