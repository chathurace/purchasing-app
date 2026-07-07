import { useMemo, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { ApiError } from "../api/client";
import {
  createConfigOption,
  deleteConfigOption,
  updateConfigOption,
  type ConfigList,
  type ConfigOption,
} from "../api/config";
import { useConfigLookup, useConfigOptionsAdmin } from "../hooks/useConfigOptions";
import { useCanManageCostCenters } from "../hooks/useCanManageCostCenters";
import { useIsAdmin } from "../hooks/useIsAdmin";
import { StorageSettings } from "../components/StorageSettings";

// SettingsPage lets admin / finance_admin manage the values that populate the
// requisition-form dropdowns. Access mirrors the Cost centers page
// (middleware.HasCostCenterAdmin enforces the same rule server-side).
export function SettingsPage() {
  const isAdmin = useIsAdmin();
  const canManage = useCanManageCostCenters();
  const { data: config } = useConfigLookup(canManage);
  const { data: options, isLoading, error } = useConfigOptionsAdmin();
  const qc = useQueryClient();
  const [actionError, setActionError] = useState<string | null>(null);

  const invalidate = () => qc.invalidateQueries({ queryKey: ["config_options"] });
  const onError = (e: unknown) =>
    setActionError(e instanceof ApiError ? e.message : "Action failed");

  const byKey = useMemo(() => {
    const m: Record<string, ConfigOption[]> = {};
    for (const o of options ?? []) (m[o.list_key] ??= []).push(o);
    for (const k of Object.keys(m)) m[k].sort((a, b) => a.sort_order - b.sort_order);
    return m;
  }, [options]);

  const create = useMutation({
    mutationFn: createConfigOption,
    onSuccess: () => {
      setActionError(null);
      invalidate();
    },
    onError,
  });
  const update = useMutation({
    mutationFn: ({ id, ...rest }: { id: number; value: string; sort_order: number; is_active: boolean }) =>
      updateConfigOption(id, rest),
    onSuccess: () => {
      setActionError(null);
      invalidate();
    },
    onError,
  });
  const remove = useMutation({
    mutationFn: (id: number) => deleteConfigOption(id),
    onSuccess: () => {
      setActionError(null);
      invalidate();
    },
    onError,
  });

  if (!canManage) {
    return (
      <div className="rounded border border-dashed bg-white p-8 text-center text-gray-500">
        You need the admin or finance_admin role to manage settings.
      </div>
    );
  }

  const lists: ConfigList[] = config?.keys ?? [];

  return (
    <div>
      <h1 className="mb-1 text-xl font-semibold text-gray-900">Settings</h1>
      <p className="mb-6 text-sm text-gray-500">
        Manage the values that appear in the new-request form dropdowns. Deactivating a value keeps
        it on existing requests but hides it from new ones; deleting removes it entirely.
      </p>

      {isAdmin && (
        <div className="mb-8">
          <StorageSettings />
        </div>
      )}

      {actionError && (
        <div className="mb-4 rounded border border-red-200 bg-red-50 px-4 py-2 text-sm text-red-700">
          {actionError}
        </div>
      )}

      {isLoading && <p className="text-gray-500">Loading…</p>}
      {error && <p className="text-red-600">Failed to load settings.</p>}

      <div className="space-y-4">
        {lists.map((list) => (
          <ListSection
            key={list.key}
            list={list}
            options={byKey[list.key] ?? []}
            adding={create.isPending}
            onAdd={(value) => {
              const max = (byKey[list.key] ?? []).reduce((m, o) => Math.max(m, o.sort_order), -1);
              create.mutate({ list_key: list.key, value, sort_order: max + 1, is_active: true });
            }}
            onRename={(o, value) =>
              update.mutate({ id: o.id, value, sort_order: o.sort_order, is_active: o.is_active })
            }
            onToggle={(o) =>
              update.mutate({ id: o.id, value: o.value, sort_order: o.sort_order, is_active: !o.is_active })
            }
            onMove={(o, dir) => {
              const arr = byKey[list.key] ?? [];
              const i = arr.findIndex((x) => x.id === o.id);
              const j = dir === "up" ? i - 1 : i + 1;
              if (j < 0 || j >= arr.length) return;
              const other = arr[j];
              // Swap the two rows' sort_order.
              update.mutate({ id: o.id, value: o.value, sort_order: other.sort_order, is_active: o.is_active });
              update.mutate({ id: other.id, value: other.value, sort_order: o.sort_order, is_active: other.is_active });
            }}
            onDelete={(o) => remove.mutate(o.id)}
          />
        ))}
      </div>
    </div>
  );
}

function ListSection({
  list,
  options,
  adding,
  onAdd,
  onRename,
  onToggle,
  onMove,
  onDelete,
}: {
  list: ConfigList;
  options: ConfigOption[];
  adding: boolean;
  onAdd: (value: string) => void;
  onRename: (o: ConfigOption, value: string) => void;
  onToggle: (o: ConfigOption) => void;
  onMove: (o: ConfigOption, dir: "up" | "down") => void;
  onDelete: (o: ConfigOption) => void;
}) {
  const [draft, setDraft] = useState("");
  const [editId, setEditId] = useState<number | null>(null);
  const [editValue, setEditValue] = useState("");

  const submitAdd = () => {
    const v = draft.trim();
    if (!v) return;
    onAdd(v);
    setDraft("");
  };

  return (
    <section className="rounded border bg-white">
      <div className="flex items-center justify-between border-b px-4 py-2.5">
        <h2 className="text-sm font-semibold text-gray-900">{list.label}</h2>
        <span className="text-xs text-gray-400">{options.length} value{options.length === 1 ? "" : "s"}</span>
      </div>

      <ul className="divide-y">
        {options.length === 0 && (
          <li className="px-4 py-3 text-sm text-gray-400">No values yet.</li>
        )}
        {options.map((o, i) => (
          <li key={o.id} className={`flex items-center gap-2 px-4 py-2 ${o.is_active ? "" : "bg-gray-50"}`}>
            <div className="flex flex-col">
              <button
                type="button"
                onClick={() => onMove(o, "up")}
                disabled={i === 0}
                className="text-gray-400 hover:text-gray-700 disabled:opacity-30"
                title="Move up"
              >
                ▲
              </button>
              <button
                type="button"
                onClick={() => onMove(o, "down")}
                disabled={i === options.length - 1}
                className="text-gray-400 hover:text-gray-700 disabled:opacity-30"
                title="Move down"
              >
                ▼
              </button>
            </div>

            {editId === o.id ? (
              <input
                autoFocus
                value={editValue}
                onChange={(e) => setEditValue(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter" && editValue.trim()) {
                    onRename(o, editValue.trim());
                    setEditId(null);
                  } else if (e.key === "Escape") setEditId(null);
                }}
                className="flex-1 rounded border px-2 py-1 text-sm"
              />
            ) : (
              <span className={`flex-1 text-sm ${o.is_active ? "text-gray-900" : "text-gray-400 line-through"}`}>
                {o.value}
              </span>
            )}

            {!o.is_active && editId !== o.id && (
              <span className="rounded bg-gray-200 px-2 py-0.5 text-[11px] font-medium text-gray-600">
                Hidden
              </span>
            )}

            <div className="flex items-center gap-1 text-xs">
              {editId === o.id ? (
                <>
                  <button
                    type="button"
                    onClick={() => {
                      if (editValue.trim()) onRename(o, editValue.trim());
                      setEditId(null);
                    }}
                    className="rounded border px-2 py-1 text-gray-700 hover:bg-gray-50"
                  >
                    Save
                  </button>
                  <button
                    type="button"
                    onClick={() => setEditId(null)}
                    className="rounded border px-2 py-1 text-gray-700 hover:bg-gray-50"
                  >
                    Cancel
                  </button>
                </>
              ) : (
                <>
                  <button
                    type="button"
                    onClick={() => {
                      setEditId(o.id);
                      setEditValue(o.value);
                    }}
                    className="rounded border px-2 py-1 text-gray-700 hover:bg-gray-50"
                  >
                    Rename
                  </button>
                  <button
                    type="button"
                    onClick={() => onToggle(o)}
                    className="rounded border px-2 py-1 text-gray-700 hover:bg-gray-50"
                  >
                    {o.is_active ? "Deactivate" : "Reactivate"}
                  </button>
                  <button
                    type="button"
                    onClick={() => onDelete(o)}
                    className="rounded border px-2 py-1 text-red-600 hover:bg-red-50"
                  >
                    Delete
                  </button>
                </>
              )}
            </div>
          </li>
        ))}
      </ul>

      <div className="flex items-center gap-2 border-t px-4 py-2.5">
        <input
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") submitAdd();
          }}
          placeholder={`Add a ${list.label.toLowerCase()} value`}
          className="flex-1 rounded border px-3 py-1.5 text-sm"
        />
        <button
          type="button"
          onClick={submitAdd}
          disabled={adding || draft.trim() === ""}
          className="rounded bg-indigo-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-indigo-700 disabled:opacity-50"
        >
          Add
        </button>
      </div>
    </section>
  );
}
