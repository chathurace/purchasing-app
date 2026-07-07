import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useUsers } from "../hooks/useUsers";
import { useIsAdmin } from "../hooks/useIsAdmin";
import { useMe } from "../hooks/useMe";
import { addUserRole, createUser, removeUserRole, setUserActive } from "../api/users";
import { ApiError } from "../api/client";
import { ASSIGNABLE_ROLES, ROLE_LABELS, type AdminUser, type Role } from "../types/api";

export function UserManagementPage() {
  const isAdmin = useIsAdmin();
  const { data: me } = useMe();
  const { data: users, isLoading, error } = useUsers(isAdmin);
  const qc = useQueryClient();

  const [email, setEmail] = useState("");
  const [name, setName] = useState("");
  const [formError, setFormError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const invalidate = () => qc.invalidateQueries({ queryKey: ["users"] });
  const onActionError = (e: unknown) =>
    setActionError(e instanceof ApiError ? e.message : "Action failed");

  const create = useMutation({
    mutationFn: () => createUser({ email: email.trim(), name: name.trim() }),
    onSuccess: () => {
      setEmail("");
      setName("");
      setFormError(null);
      invalidate();
    },
    onError: (e) => setFormError(e instanceof ApiError ? e.message : "Failed to add user"),
  });

  const addRole = useMutation({
    mutationFn: ({ id, role }: { id: number; role: Role }) => addUserRole(id, role),
    onSuccess: invalidate,
    onError: onActionError,
  });
  const removeRole = useMutation({
    mutationFn: ({ id, role }: { id: number; role: Role }) => removeUserRole(id, role),
    onSuccess: invalidate,
    onError: onActionError,
  });
  const toggleActive = useMutation({
    mutationFn: ({ id, active }: { id: number; active: boolean }) => setUserActive(id, active),
    onSuccess: invalidate,
    onError: onActionError,
  });

  if (!isAdmin) {
    return (
      <div className="rounded border border-dashed bg-white p-8 text-center text-gray-500">
        You need the admin role to manage users.
      </div>
    );
  }

  return (
    <div>
      <h1 className="mb-1 text-xl font-semibold text-gray-900">Users</h1>
      <p className="mb-6 text-sm text-gray-500">
        Add people by email and manage their roles. Anyone can sign in and starts as staff; adding
        them here lets you assign roles before their first login. Deactivating a user blocks them
        from signing in without losing their history.
      </p>

      {/* Add user */}
      <form
        onSubmit={(e) => {
          e.preventDefault();
          create.mutate();
        }}
        className="mb-6 rounded border bg-white p-4"
      >
        <div className="flex flex-wrap items-end gap-3">
          <label className="flex flex-col text-sm">
            <span className="mb-1 font-medium text-gray-700">Email</span>
            <input
              type="email"
              required
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="person@company.com"
              className="w-64 rounded border px-3 py-1.5 text-sm"
            />
          </label>
          <label className="flex flex-col text-sm">
            <span className="mb-1 font-medium text-gray-700">Name (optional)</span>
            <input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Jane Doe"
              className="w-48 rounded border px-3 py-1.5 text-sm"
            />
          </label>
          <button
            type="submit"
            disabled={create.isPending || email.trim() === ""}
            className="rounded bg-indigo-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-indigo-700 disabled:opacity-50"
          >
            {create.isPending ? "Adding…" : "Add user"}
          </button>
        </div>
        {formError && <p className="mt-2 text-sm text-red-600">{formError}</p>}
      </form>

      {actionError && (
        <div className="mb-4 rounded border border-red-200 bg-red-50 px-4 py-2 text-sm text-red-700">
          {actionError}
        </div>
      )}

      {isLoading && <p className="text-gray-500">Loading…</p>}
      {error && <p className="text-red-600">Failed to load users.</p>}

      {users && (
        <div className="overflow-hidden rounded border bg-white">
          <table className="w-full text-sm">
            <thead className="border-b bg-gray-50 text-left text-gray-500">
              <tr>
                <th className="px-4 py-2 font-medium">User</th>
                <th className="px-4 py-2 font-medium">Status</th>
                <th className="px-4 py-2 font-medium">Roles</th>
                <th className="px-4 py-2 font-medium text-right">Actions</th>
              </tr>
            </thead>
            <tbody>
              {users.map((u) => (
                <UserRow
                  key={u.id}
                  user={u}
                  isSelf={me?.id === u.id}
                  onAddRole={(role) => addRole.mutate({ id: u.id, role })}
                  onRemoveRole={(role) => removeRole.mutate({ id: u.id, role })}
                  onToggleActive={() => toggleActive.mutate({ id: u.id, active: !u.is_active })}
                />
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

function UserRow({
  user,
  isSelf,
  onAddRole,
  onRemoveRole,
  onToggleActive,
}: {
  user: AdminUser;
  isSelf: boolean;
  onAddRole: (role: Role) => void;
  onRemoveRole: (role: Role) => void;
  onToggleActive: () => void;
}) {
  const addable = ASSIGNABLE_ROLES.filter((r) => !user.roles.includes(r));

  return (
    <tr className={`border-b last:border-0 ${user.is_active ? "" : "bg-gray-50"}`}>
      <td className="px-4 py-3 align-top">
        <div className="font-medium text-gray-900">{user.email || "(no email)"}</div>
        {user.name && <div className="text-gray-500">{user.name}</div>}
      </td>
      <td className="px-4 py-3 align-top">
        {!user.is_active ? (
          <span className="rounded bg-red-100 px-2 py-0.5 text-xs font-medium text-red-700">
            Deactivated
          </span>
        ) : user.pending ? (
          <span className="rounded bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-700">
            Invited
          </span>
        ) : (
          <span className="rounded bg-green-100 px-2 py-0.5 text-xs font-medium text-green-700">
            Active
          </span>
        )}
      </td>
      <td className="px-4 py-3 align-top">
        <div className="flex flex-wrap items-center gap-1.5">
          {user.roles.length === 0 && <span className="text-gray-400">—</span>}
          {user.roles.map((r) => (
            <span
              key={r}
              className="inline-flex items-center gap-1 rounded bg-gray-100 px-2 py-0.5 text-xs text-gray-700"
            >
              {ROLE_LABELS[r] ?? r}
              {r !== "staff" && (
                <button
                  type="button"
                  onClick={() => onRemoveRole(r)}
                  title={`Remove ${ROLE_LABELS[r] ?? r}`}
                  className="text-gray-400 hover:text-red-600"
                >
                  ×
                </button>
              )}
            </span>
          ))}
          {addable.length > 0 && (
            <select
              value=""
              onChange={(e) => {
                if (e.target.value) onAddRole(e.target.value as Role);
                e.target.value = "";
              }}
              className="rounded border border-dashed px-2 py-0.5 text-xs text-gray-600"
            >
              <option value="">+ Add role</option>
              {addable.map((r) => (
                <option key={r} value={r}>
                  {ROLE_LABELS[r] ?? r}
                </option>
              ))}
            </select>
          )}
        </div>
      </td>
      <td className="px-4 py-3 text-right align-top">
        <button
          type="button"
          onClick={onToggleActive}
          disabled={isSelf}
          title={isSelf ? "You cannot deactivate your own account" : undefined}
          className="rounded border px-3 py-1 text-xs text-gray-700 hover:bg-gray-50 disabled:opacity-40"
        >
          {user.is_active ? "Deactivate" : "Reactivate"}
        </button>
      </td>
    </tr>
  );
}
