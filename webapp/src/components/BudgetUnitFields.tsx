import { useMemo } from "react";
import { useUserLookup } from "../hooks/useUserLookup";
import { ApproverPicker } from "./ApproverPicker";
import { CurrencyInput } from "./CurrencyInput";
import { NumberInput } from "./NumberInput";
import type { BudgetUnitBracketInput, BudgetUnitInput, UserSummary } from "../types/api";

interface Props {
  value: BudgetUnitInput;
  onChange: (next: BudgetUnitInput) => void;
}

const baseInputCls = "rounded border px-3 py-2 text-sm focus:border-indigo-500 focus:outline-none";
const inputCls = "w-full " + baseInputCls;
const labelCls = "block text-sm font-medium text-gray-700 mb-1";

// BudgetUnitFields edits everything except the active/inactive status, which is
// controlled separately via an Activate/Deactivate action (mirrors VendorFields).
// The approval brackets are an ordered list: for a given estimated value the
// first bracket whose [min, max] contains it wins, and any of that bracket's
// approvers may sign off the budget card.
export function BudgetUnitFields({ value, onChange }: Props) {
  const set = (patch: Partial<BudgetUnitInput>) => onChange({ ...value, ...patch });
  const { data: users, isLoading: usersLoading } = useUserLookup();

  const setBracket = (i: number, patch: Partial<BudgetUnitBracketInput>) => {
    const brackets = value.brackets.map((b, idx) => (idx === i ? { ...b, ...patch } : b));
    set({ brackets });
  };
  const addBracket = () =>
    set({
      brackets: [
        ...value.brackets,
        { currency: value.currency, min_value: 0, max_value: null, approver_ids: [] },
      ],
    });
  const removeBracket = (i: number) =>
    set({ brackets: value.brackets.filter((_, idx) => idx !== i) });
  const moveBracket = (i: number, dir: -1 | 1) => {
    const j = i + dir;
    if (j < 0 || j >= value.brackets.length) return;
    const brackets = [...value.brackets];
    [brackets[i], brackets[j]] = [brackets[j], brackets[i]];
    set({ brackets });
  };

  return (
    <div className="space-y-4">
      <div className="flex gap-3">
        <div className="w-40 shrink-0">
          <label className={labelCls}>Code</label>
          <input
            className={inputCls}
            value={value.code}
            onChange={(e) => set({ code: e.target.value })}
            placeholder="e.g. BU-ENG-001"
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
          placeholder="What this budget unit covers"
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
          <label className={labelCls}>Budget currency</label>
          <CurrencyInput
            className={inputCls}
            value={value.currency}
            onChange={(currency) => set({ currency })}
          />
        </div>
      </div>

      <div>
        <label className={labelCls}>Default approver *</label>
        <select
          className={inputCls}
          value={value.default_approver_id ?? ""}
          onChange={(e) => set({ default_approver_id: e.target.value === "" ? null : Number(e.target.value) })}
        >
          <option value="">{usersLoading ? "Loading users…" : "— Select —"}</option>
          {(users ?? []).map((u) => (
            <option key={u.id} value={u.id}>
              {u.name ? `${u.name} (${u.email})` : u.email}
            </option>
          ))}
        </select>
        <p className="mt-1 text-xs text-gray-500">
          Approves any request that matches no bracket (no value, another currency, or out of range).
        </p>
      </div>

      <div>
        <div className="mb-1 flex items-center justify-between">
          <label className="block text-sm font-medium text-gray-700">Approval brackets (optional)</label>
          <button
            type="button"
            onClick={addBracket}
            className="rounded border border-indigo-200 px-2 py-1 text-xs font-medium text-indigo-700 hover:bg-indigo-50"
          >
            + Add bracket
          </button>
        </div>
        <p className="mb-2 text-xs text-gray-500">
          Brackets are optional. When set, the estimated value + currency select the bracket (matching currency
          and min–max range); if several match, the first listed wins, and any approver of the selected bracket
          may approve. Anything with no match — and <strong>every</strong> request when no brackets are configured —
          goes to the default approver above.
        </p>
        {value.brackets.length === 0 && (
          <p className="rounded border border-dashed border-gray-300 px-3 py-4 text-center text-sm text-gray-400">
            No brackets — the default approver handles every request. Add a bracket to route higher-value
            requests to different approvers.
          </p>
        )}
        <div className="space-y-3">
          {value.brackets.map((b, i) => (
            <BracketRow
              key={i}
              index={i}
              bracket={b}
              users={users ?? []}
              usersLoading={usersLoading}
              isFirst={i === 0}
              isLast={i === value.brackets.length - 1}
              onChange={(patch) => setBracket(i, patch)}
              onRemove={() => removeBracket(i)}
              onMove={(dir) => moveBracket(i, dir)}
            />
          ))}
        </div>
      </div>
    </div>
  );
}

function BracketRow({
  index,
  bracket,
  users,
  usersLoading,
  isFirst,
  isLast,
  onChange,
  onRemove,
  onMove,
}: {
  index: number;
  bracket: BudgetUnitBracketInput;
  users: UserSummary[];
  usersLoading: boolean;
  isFirst: boolean;
  isLast: boolean;
  onChange: (patch: Partial<BudgetUnitBracketInput>) => void;
  onRemove: () => void;
  onMove: (dir: -1 | 1) => void;
}) {
  const unbounded = bracket.max_value == null;
  const selectedUsers = useMemo(
    () => users.filter((u) => bracket.approver_ids.includes(u.id)),
    [users, bracket.approver_ids],
  );

  return (
    <div className="rounded border border-gray-200 p-3">
      <div className="mb-2 flex items-center justify-between">
        <span className="text-xs font-semibold uppercase tracking-wide text-gray-500">
          Bracket {index + 1}
        </span>
        <div className="flex items-center gap-1">
          <button
            type="button"
            disabled={isFirst}
            onClick={() => onMove(-1)}
            className="rounded border px-2 py-0.5 text-xs text-gray-600 hover:bg-gray-50 disabled:opacity-30"
            title="Move up"
          >
            ↑
          </button>
          <button
            type="button"
            disabled={isLast}
            onClick={() => onMove(1)}
            className="rounded border px-2 py-0.5 text-xs text-gray-600 hover:bg-gray-50 disabled:opacity-30"
            title="Move down"
          >
            ↓
          </button>
          <button
            type="button"
            onClick={onRemove}
            className="rounded border border-red-200 px-2 py-0.5 text-xs text-red-600 hover:bg-red-50"
          >
            Remove
          </button>
        </div>
      </div>
      <div className="flex flex-wrap items-end gap-3">
        <div className="w-24">
          <label className={labelCls}>Currency *</label>
          <CurrencyInput
            className={inputCls}
            value={bracket.currency}
            onChange={(currency) => onChange({ currency })}
            placeholder="USD"
          />
        </div>
        <div className="w-32">
          <label className={labelCls}>Min value</label>
          <NumberInput
            min={0}
            step="any"
            className={inputCls}
            value={bracket.min_value}
            onChange={(min_value) => onChange({ min_value })}
          />
        </div>
        <div className="w-32">
          <label className={labelCls}>Max value</label>
          <NumberInput
            min={0}
            step="any"
            disabled={unbounded}
            className={inputCls + (unbounded ? " bg-gray-100 text-gray-400" : "")}
            value={bracket.max_value ?? 0}
            onChange={(max_value) => onChange({ max_value })}
          />
        </div>
        <label className="mb-2 flex items-center gap-1.5 text-sm text-gray-600">
          <input
            type="checkbox"
            className="accent-indigo-600"
            checked={unbounded}
            onChange={(e) => onChange({ max_value: e.target.checked ? null : 0 })}
          />
          No upper limit
        </label>
      </div>
      <div className="mt-3">
        <label className={labelCls}>Approvers *</label>
        <ApproverPicker
          candidates={users}
          selectedIds={bracket.approver_ids}
          selectedUsers={selectedUsers}
          onAdd={(u) => onChange({ approver_ids: [...bracket.approver_ids, u.id] })}
          onRemove={(id) => onChange({ approver_ids: bracket.approver_ids.filter((x) => x !== id) })}
          loading={usersLoading}
        />
      </div>
    </div>
  );
}
