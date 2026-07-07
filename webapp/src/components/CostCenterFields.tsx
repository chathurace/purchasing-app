import { useMemo } from "react";
import { useUserLookup } from "../hooks/useUserLookup";
import { ApproverPicker } from "./ApproverPicker";
import { NumberInput } from "./NumberInput";
import type { CostCenterInput, UserSummary } from "../types/api";

interface Props {
  value: CostCenterInput;
  onChange: (next: CostCenterInput) => void;
}

const baseInputCls = "rounded border px-3 py-2 text-sm focus:border-indigo-500 focus:outline-none";
const inputCls = "w-full " + baseInputCls;
const labelCls = "block text-sm font-medium text-gray-700 mb-1";

// CostCenterFields edits everything except the active/inactive status, which is
// controlled separately via an Activate/Deactivate action (mirrors VendorFields).
export function CostCenterFields({ value, onChange }: Props) {
  const set = (patch: Partial<CostCenterInput>) => onChange({ ...value, ...patch });
  const { data: users, isLoading: usersLoading } = useUserLookup();

  // Secondary-owner candidates: every active user except the primary owner.
  const candidates = useMemo(
    () => (users ?? []).filter((u) => u.id !== value.primary_owner_id),
    [users, value.primary_owner_id],
  );

  const selectedSecondary = useMemo(
    () => (users ?? []).filter((u) => value.secondary_owner_ids.includes(u.id)),
    [users, value.secondary_owner_ids],
  );

  const addSecondary = (u: UserSummary) =>
    set({ secondary_owner_ids: [...value.secondary_owner_ids, u.id] });
  const removeSecondary = (id: number) =>
    set({ secondary_owner_ids: value.secondary_owner_ids.filter((x) => x !== id) });

  return (
    <div className="space-y-4">
      <div className="flex gap-3">
        <div className="w-40 shrink-0">
          <label className={labelCls}>Code</label>
          <input
            className={inputCls}
            value={value.code}
            onChange={(e) => set({ code: e.target.value })}
            placeholder="e.g. CC-ENG-001"
          />
        </div>
        <div className="flex-1">
          <label className={labelCls}>Name *</label>
          <input
            className={inputCls}
            value={value.name}
            onChange={(e) => set({ name: e.target.value })}
            placeholder="e.g. Engineering"
          />
        </div>
      </div>

      <div>
        <label className={labelCls}>Description</label>
        <textarea
          className={inputCls}
          rows={2}
          value={value.description}
          onChange={(e) => set({ description: e.target.value })}
          placeholder="What this cost center covers"
        />
      </div>

      <div>
        <label className={labelCls}>Primary owner</label>
        <select
          className={inputCls}
          value={value.primary_owner_id ?? ""}
          onChange={(e) => {
            const id = e.target.value === "" ? null : Number(e.target.value);
            // Drop the chosen primary owner from secondary owners if present.
            set({
              primary_owner_id: id,
              secondary_owner_ids: value.secondary_owner_ids.filter((x) => x !== id),
            });
          }}
        >
          <option value="">{usersLoading ? "Loading users…" : "— None —"}</option>
          {(users ?? []).map((u) => (
            <option key={u.id} value={u.id}>
              {u.name ? `${u.name} (${u.email})` : u.email}
            </option>
          ))}
        </select>
      </div>

      <div>
        <label className={labelCls}>Secondary owners</label>
        <ApproverPicker
          candidates={candidates}
          selectedIds={value.secondary_owner_ids}
          selectedUsers={selectedSecondary}
          onAdd={addSecondary}
          onRemove={removeSecondary}
          loading={usersLoading}
        />
      </div>

      <div className="flex gap-3">
        <div className="flex-1">
          <label className={labelCls}>Budget</label>
          <NumberInput
            min={0}
            step="any"
            className={inputCls}
            value={value.budget}
            onChange={(budget) => set({ budget })}
            placeholder="0.00"
          />
        </div>
        <div className="w-32 shrink-0">
          <label className={labelCls}>Currency</label>
          <input
            className={inputCls}
            value={value.currency}
            onChange={(e) => set({ currency: e.target.value })}
            placeholder="e.g. USD"
          />
        </div>
      </div>
    </div>
  );
}
