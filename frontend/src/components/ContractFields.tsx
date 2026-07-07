import { NumberInput } from "./NumberInput";
import type { ContractInput } from "../types/api";

interface Props {
  value: ContractInput;
  onChange: (next: ContractInput) => void;
}

const inputCls = "w-full rounded border px-3 py-2 text-sm focus:border-indigo-500 focus:outline-none";
const labelCls = "block text-sm font-medium text-gray-700 mb-1";

export function ContractFields({ value, onChange }: Props) {
  const set = (patch: Partial<ContractInput>) => onChange({ ...value, ...patch });

  return (
    <div className="space-y-5">
      <div>
        <label className={labelCls}>Title</label>
        <input
          className={inputCls}
          value={value.title}
          onChange={(e) => set({ title: e.target.value })}
          placeholder="Contract title"
        />
      </div>

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
      </div>

      <div>
        <label className={labelCls}>Terms</label>
        <textarea
          className={inputCls}
          rows={5}
          value={value.terms}
          onChange={(e) => set({ terms: e.target.value })}
          placeholder="Key contract terms"
        />
      </div>
    </div>
  );
}
