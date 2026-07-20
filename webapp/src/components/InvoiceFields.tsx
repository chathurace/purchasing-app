import {
  allocationsSum,
  allocationsValid,
  formatMoney,
  invoiceEffectiveTotal,
  invoiceItemsTotal,
} from "../types/api";
import type { AllocationMode, CostAllocationInput, InvoiceInput } from "../types/api";
import { useBudgetUnitLookup } from "../hooks/useBudgetUnits";
import { NullableNumberInput, NumberInput } from "./NumberInput";
import { CurrencyInput } from "./CurrencyInput";

interface Props {
  value: InvoiceInput;
  onChange: (next: InvoiceInput) => void;
}

const baseInputCls = "rounded border px-3 py-2 text-sm focus:border-indigo-500 focus:outline-none";
const inputCls = "w-full " + baseInputCls;
const labelCls = "block text-sm font-medium text-gray-700 mb-1";

type IItem = { description: string; quantity: number; unit_price: number };

export function InvoiceFields({ value, onChange }: Props) {
  const set = (patch: Partial<InvoiceInput>) => onChange({ ...value, ...patch });
  const { data: budgetUnits, isLoading: buLoading } = useBudgetUnitLookup();

  const setItem = (i: number, patch: Partial<IItem>) => {
    const items = value.items.map((it, idx) => (idx === i ? { ...it, ...patch } : it));
    set({ items });
  };

  // itemsTotal is the sum of the line items; total is the effective invoice
  // value (the entered total when given, else itemsTotal). The two can diverge —
  // the entered total wins and we only warn.
  const itemsTotal = invoiceItemsTotal(value.items);
  const total = invoiceEffectiveTotal(value);
  const hasEnteredTotal = value.entered_total != null;
  const mismatch =
    hasEnteredTotal && value.items.length > 0 && Math.abs((value.entered_total ?? 0) - itemsTotal) > 0.01;

  // --- budget-unit allocation helpers ---
  const allocs = value.cost_allocations;
  const setAlloc = (i: number, patch: Partial<CostAllocationInput>) =>
    set({ cost_allocations: allocs.map((a, idx) => (idx === i ? { ...a, ...patch } : a)) });
  const addAlloc = () =>
    set({ cost_allocations: [...allocs, { budget_unit_id: 0, value: 0 }] });
  const removeAlloc = (i: number) =>
    set({ cost_allocations: allocs.filter((_, idx) => idx !== i) });
  const setMode = (mode: AllocationMode) => set({ allocation_mode: mode });

  const allocSum = allocationsSum(allocs);
  const allocTarget = value.allocation_mode === "percentage" ? 100 : total;
  const allocOk = allocationsValid(value.allocation_mode, allocs, total);
  // Budget units already chosen on other rows, to avoid picking duplicates.
  const chosenIds = (i: number) =>
    new Set(allocs.filter((_, idx) => idx !== i).map((a) => a.budget_unit_id));

  return (
    <div className="space-y-5">
      <div className="flex gap-2">
        <div className="flex-1">
          <label className={labelCls}>Vendor invoice no.</label>
          <input
            className={inputCls}
            value={value.vendor_invoice_no}
            onChange={(e) => set({ vendor_invoice_no: e.target.value })}
            placeholder="The vendor's own invoice number"
          />
        </div>
        <div className="w-28 shrink-0">
          <label className={labelCls}>Currency</label>
          <CurrencyInput
            className={inputCls}
            value={value.currency}
            maxLength={3}
            onChange={(currency) => set({ currency })}
            placeholder="USD"
          />
        </div>
      </div>

      <div className="flex gap-2">
        <div className="w-44 shrink-0">
          <label className={labelCls}>Invoice date</label>
          <input
            type="date"
            className={inputCls}
            value={value.invoice_date}
            onChange={(e) => set({ invoice_date: e.target.value })}
          />
        </div>
        <div className="w-44 shrink-0">
          <label className={labelCls}>Due date</label>
          <input
            type="date"
            className={inputCls}
            value={value.due_date ?? ""}
            onChange={(e) => set({ due_date: e.target.value || null })}
          />
        </div>
      </div>

      <div>
        <div className="mb-1 flex items-center justify-between">
          <label className={labelCls}>Line items</label>
          <button
            type="button"
            className="text-sm text-indigo-600"
            onClick={() => set({ items: [...value.items, { description: "", quantity: 1, unit_price: 0 }] })}
          >
            + Add line
          </button>
        </div>
        <div className="space-y-2">
          {value.items.length === 0 && <p className="text-sm text-gray-400">No line items.</p>}
          {value.items.map((it, i) => (
            <div key={i} className="flex gap-2">
              <input
                className={baseInputCls + " min-w-0 flex-1"}
                value={it.description}
                onChange={(e) => setItem(i, { description: e.target.value })}
                placeholder="Description"
              />
              <NumberInput
                min={0}
                step="any"
                className={baseInputCls + " w-20 shrink-0"}
                value={it.quantity}
                onChange={(quantity) => setItem(i, { quantity })}
                placeholder="Qty"
              />
              <NumberInput
                min={0}
                step="any"
                className={baseInputCls + " w-28 shrink-0"}
                value={it.unit_price}
                onChange={(unit_price) => setItem(i, { unit_price })}
                placeholder="Unit price"
              />
              <button
                type="button"
                className="rounded border px-2 text-sm text-gray-500 hover:bg-gray-50"
                onClick={() => set({ items: value.items.filter((_, idx) => idx !== i) })}
              >
                Remove
              </button>
            </div>
          ))}
        </div>
        <p className="mt-2 text-right text-sm text-gray-600">
          Line items total:{" "}
          <span className="font-medium text-gray-900">{formatMoney(itemsTotal, value.currency)}</span>
        </p>
      </div>

      {/* Invoice total — may be entered directly; takes priority over the line items. */}
      <div>
        <label className={labelCls}>Invoice total</label>
        <div className="flex items-center gap-2">
          <div className="relative w-44">
            <NullableNumberInput
              min={0}
              step="any"
              className={baseInputCls + " w-full pr-12"}
              value={value.entered_total}
              onChange={(entered_total) => set({ entered_total })}
              placeholder={itemsTotal.toFixed(2)}
            />
            <span className="pointer-events-none absolute right-2 top-1/2 -translate-y-1/2 text-xs text-gray-400">
              {value.currency}
            </span>
          </div>
          {hasEnteredTotal && (
            <button
              type="button"
              className="text-sm text-indigo-600"
              onClick={() => set({ entered_total: null })}
            >
              Use line items total
            </button>
          )}
        </div>
        <p className="mt-1 text-xs text-gray-400">
          Leave blank to use the line items total. When set, this value is the invoice total.
        </p>
        {mismatch && (
          <p className="mt-1 text-sm text-amber-600">
            Entered total ({formatMoney(value.entered_total ?? 0, value.currency)}) differs from the line items
            total ({formatMoney(itemsTotal, value.currency)}). The entered total will be used.
          </p>
        )}
      </div>

      {/* Budget-unit allocation */}
      <div>
        <div className="mb-1 flex items-center justify-between">
          <label className={labelCls}>Budget units</label>
          <div className="flex items-center gap-3 text-sm">
            <label className="flex items-center gap-1 text-gray-600">
              <input
                type="radio"
                name="allocation_mode"
                checked={value.allocation_mode === "percentage"}
                onChange={() => setMode("percentage")}
              />
              Percentage
            </label>
            <label className="flex items-center gap-1 text-gray-600">
              <input
                type="radio"
                name="allocation_mode"
                checked={value.allocation_mode === "amount"}
                onChange={() => setMode("amount")}
              />
              Amount
            </label>
            <button type="button" className="text-indigo-600" onClick={addAlloc}>
              + Add budget unit
            </button>
          </div>
        </div>
        <p className="mb-2 text-xs text-gray-400">
          Split this invoice across one or more budget units
          {value.allocation_mode === "percentage" ? " by percentage (must total 100%)" : " by amount (must total the invoice total)"}.
        </p>
        <div className="space-y-2">
          {allocs.length === 0 && <p className="text-sm text-gray-400">No budget units assigned.</p>}
          {allocs.map((a, i) => {
            const taken = chosenIds(i);
            const resolved =
              value.allocation_mode === "percentage"
                ? (total * (a.value || 0)) / 100
                : total > 0
                  ? ((a.value || 0) / total) * 100
                  : 0;
            return (
              <div key={i} className="flex items-center gap-2">
                <select
                  className={baseInputCls + " min-w-0 flex-1"}
                  value={a.budget_unit_id || ""}
                  onChange={(e) => setAlloc(i, { budget_unit_id: Number(e.target.value) })}
                >
                  <option value="">{buLoading ? "Loading…" : "Select budget unit"}</option>
                  {(budgetUnits ?? [])
                    .filter((c) => c.id === a.budget_unit_id || !taken.has(c.id))
                    .map((c) => (
                      <option key={c.id} value={c.id}>
                        {c.code ? `${c.code} — ${c.name}` : c.name}
                      </option>
                    ))}
                </select>
                <div className="relative w-32 shrink-0">
                  <NumberInput
                    min={0}
                    step="any"
                    className={baseInputCls + " w-full pr-7"}
                    value={a.value}
                    onChange={(val) => setAlloc(i, { value: val })}
                  />
                  <span className="pointer-events-none absolute right-2 top-1/2 -translate-y-1/2 text-xs text-gray-400">
                    {value.allocation_mode === "percentage" ? "%" : value.currency}
                  </span>
                </div>
                <span className="w-36 shrink-0 text-right text-xs text-gray-500">
                  {value.allocation_mode === "percentage"
                    ? `= ${formatMoney(resolved, value.currency)}`
                    : `= ${resolved.toFixed(1)}%`}
                </span>
                <button
                  type="button"
                  className="rounded border px-2 text-sm text-gray-500 hover:bg-gray-50"
                  onClick={() => removeAlloc(i)}
                >
                  Remove
                </button>
              </div>
            );
          })}
        </div>
        {allocs.length > 0 && (
          <p className={`mt-2 text-right text-sm ${allocOk ? "text-gray-600" : "text-red-600"}`}>
            Allocated:{" "}
            <span className="font-medium">
              {value.allocation_mode === "percentage"
                ? `${allocSum.toFixed(2)}% / 100%`
                : `${formatMoney(allocSum, value.currency)} / ${formatMoney(allocTarget, value.currency)}`}
            </span>
            {!allocOk && <span className="ml-2">— must add up before saving</span>}
          </p>
        )}
      </div>

      <div>
        <label className={labelCls}>Note</label>
        <textarea
          className={inputCls}
          rows={3}
          value={value.note}
          onChange={(e) => set({ note: e.target.value })}
          placeholder="Any additional context"
        />
      </div>
    </div>
  );
}
