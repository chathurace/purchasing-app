import { VendorSelect } from "./VendorSelect";
import { NumberInput } from "./NumberInput";
import type { QuotationInput } from "../types/api";

interface Props {
  value: QuotationInput;
  onChange: (next: QuotationInput) => void;
}

const baseInputCls = "rounded border px-3 py-2 text-sm focus:border-indigo-500 focus:outline-none";
const inputCls = "w-full " + baseInputCls;
const labelCls = "block text-sm font-medium text-gray-700 mb-1";

type QItem = { description: string; quantity: number; unit_price: number };

export function QuotationFields({ value, onChange }: Props) {
  const set = (patch: Partial<QuotationInput>) => onChange({ ...value, ...patch });

  const setItem = (i: number, patch: Partial<QItem>) => {
    const items = value.items.map((it, idx) => (idx === i ? { ...it, ...patch } : it));
    set({ items });
  };

  return (
    <div className="space-y-5">
      <VendorSelect value={value.vendor_id} onChange={(vendor_id) => set({ vendor_id })} />

      <div className="flex gap-2">
        <div className="flex-1">
          <label className={labelCls}>Total amount</label>
          <NumberInput
            min={0}
            step="any"
            className={inputCls}
            value={value.total_amount}
            onChange={(total_amount) => set({ total_amount })}
          />
        </div>
        <div className="w-28 shrink-0">
          <label className={labelCls}>Currency</label>
          <input
            className={inputCls}
            value={value.currency}
            maxLength={3}
            onChange={(e) => set({ currency: e.target.value.toUpperCase() })}
            placeholder="USD"
          />
        </div>
        <div className="w-44 shrink-0">
          <label className={labelCls}>Valid until</label>
          <input
            type="date"
            className={inputCls}
            value={value.valid_until ?? ""}
            onChange={(e) => set({ valid_until: e.target.value || null })}
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
      </div>

      <div>
        <label className={labelCls}>Notes</label>
        <textarea
          className={inputCls}
          rows={3}
          value={value.notes}
          onChange={(e) => set({ notes: e.target.value })}
          placeholder="Any additional context"
        />
      </div>
    </div>
  );
}
