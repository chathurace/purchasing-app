import { NumberInput } from "./NumberInput";
import type { GRNInput } from "../types/api";

interface Props {
  value: GRNInput;
  onChange: (next: GRNInput) => void;
}

const baseInputCls = "rounded border px-3 py-2 text-sm focus:border-indigo-500 focus:outline-none";
const inputCls = "w-full " + baseInputCls;
const labelCls = "block text-sm font-medium text-gray-700 mb-1";

type GItem = { description: string; quantity: number };

export function GRNFields({ value, onChange }: Props) {
  const set = (patch: Partial<GRNInput>) => onChange({ ...value, ...patch });

  const setItem = (i: number, patch: Partial<GItem>) => {
    const items = value.items.map((it, idx) => (idx === i ? { ...it, ...patch } : it));
    set({ items });
  };

  return (
    <div className="space-y-5">
      <div className="flex gap-2">
        <div className="w-44 shrink-0">
          <label className={labelCls}>Received date</label>
          <input
            type="date"
            className={inputCls}
            value={value.received_date}
            onChange={(e) => set({ received_date: e.target.value })}
          />
        </div>
        <div className="flex-1">
          <label className={labelCls}>Received by</label>
          <input
            className={inputCls}
            value={value.received_by}
            onChange={(e) => set({ received_by: e.target.value })}
            placeholder="Name of the person who received the goods"
          />
        </div>
      </div>

      <div>
        <div className="mb-1 flex items-center justify-between">
          <label className={labelCls}>Items received</label>
          <button
            type="button"
            className="text-sm text-indigo-600"
            onClick={() => set({ items: [...value.items, { description: "", quantity: 1 }] })}
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
                className={baseInputCls + " w-24 shrink-0"}
                value={it.quantity}
                onChange={(quantity) => setItem(i, { quantity })}
                placeholder="Qty"
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
        <label className={labelCls}>Note</label>
        <textarea
          className={inputCls}
          rows={3}
          value={value.note}
          onChange={(e) => set({ note: e.target.value })}
          placeholder="Condition, discrepancies, delivery reference…"
        />
      </div>
    </div>
  );
}
