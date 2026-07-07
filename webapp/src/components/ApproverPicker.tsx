import { useMemo, useState } from "react";
import type { UserSummary } from "../types/api";

interface Props {
  // candidates is the full set of selectable users (already excluding self).
  candidates: UserSummary[];
  // selectedIds are the users already chosen/added; they are hidden from the
  // add list to avoid duplicates.
  selectedIds: number[];
  onAdd: (user: UserSummary) => void;
  // When provided, the chosen users render as removable chips below the input.
  selectedUsers?: UserSummary[];
  onRemove?: (id: number) => void;
  disabled?: boolean;
  loading?: boolean;
}

function label(u: UserSummary): string {
  return u.name ? `${u.name} (${u.email})` : u.email || `#${u.id}`;
}

// ApproverPicker is a search-and-add control for choosing approvers. It is used
// both on the new-request form (local selection) and on the detail page (each
// add is an immediate API call).
export function ApproverPicker({
  candidates,
  selectedIds,
  onAdd,
  selectedUsers,
  onRemove,
  disabled,
  loading,
}: Props) {
  const [query, setQuery] = useState("");

  const available = useMemo(() => {
    const chosen = new Set(selectedIds);
    const q = query.trim().toLowerCase();
    return candidates
      .filter((u) => !chosen.has(u.id))
      .filter((u) => q === "" || u.email.toLowerCase().includes(q) || u.name.toLowerCase().includes(q))
      .slice(0, 8);
  }, [candidates, selectedIds, query]);

  return (
    <div>
      {onRemove && selectedUsers && selectedUsers.length > 0 && (
        <ul className="mb-2 flex flex-wrap gap-2">
          {selectedUsers.map((u) => (
            <li
              key={u.id}
              className="flex items-center gap-1 rounded-full bg-indigo-50 px-2.5 py-1 text-xs text-indigo-700"
            >
              {label(u)}
              <button
                type="button"
                disabled={disabled}
                className="text-indigo-400 hover:text-red-600 disabled:opacity-50"
                onClick={() => onRemove(u.id)}
                aria-label={`Remove ${label(u)}`}
              >
                ×
              </button>
            </li>
          ))}
        </ul>
      )}

      <input
        className="w-full rounded border px-3 py-2 text-sm focus:border-indigo-500 focus:outline-none disabled:bg-gray-50"
        value={query}
        disabled={disabled}
        onChange={(e) => setQuery(e.target.value)}
        placeholder={loading ? "Loading users…" : "Search people by name or email…"}
      />

      {query.trim() !== "" && (
        <ul className="mt-1 max-h-56 divide-y overflow-auto rounded border">
          {available.length === 0 ? (
            <li className="px-3 py-2 text-sm text-gray-400">No matching users.</li>
          ) : (
            available.map((u) => (
              <li key={u.id}>
                <button
                  type="button"
                  disabled={disabled}
                  className="flex w-full items-center justify-between px-3 py-2 text-left text-sm hover:bg-indigo-50 disabled:opacity-50"
                  onClick={() => {
                    onAdd(u);
                    setQuery("");
                  }}
                >
                  <span className="text-gray-800">{u.name || u.email}</span>
                  {u.name && <span className="text-xs text-gray-400">{u.email}</span>}
                </button>
              </li>
            ))
          )}
        </ul>
      )}
    </div>
  );
}
