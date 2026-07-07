import type { VendorInput } from "../types/api";

// A fresh, active vendor draft. Shared by the management page and the inline
// VendorSelect quick-add form.
export const emptyVendor: VendorInput = {
  name: "",
  contact_name: "",
  email: "",
  phone: "",
  notes: "",
  is_active: true,
  tax_id: "",
  address_line: "",
  city: "",
  postal_code: "",
  country: "",
  website: "",
  registered: false,
};

interface Props {
  value: VendorInput;
  onChange: (next: VendorInput) => void;
}

const baseInputCls = "rounded border px-3 py-2 text-sm focus:border-indigo-500 focus:outline-none";
const inputCls = "w-full " + baseInputCls;
const labelCls = "block text-sm font-medium text-gray-700 mb-1";

// VendorFields edits everything except the active/inactive status, which is
// controlled separately via an Activate/Deactivate action.
export function VendorFields({ value, onChange }: Props) {
  const set = (patch: Partial<VendorInput>) => onChange({ ...value, ...patch });

  return (
    <div className="space-y-4">
      <div>
        <label className={labelCls}>Name *</label>
        <input
          className={inputCls}
          value={value.name}
          onChange={(e) => set({ name: e.target.value })}
          placeholder="Vendor / company name"
        />
      </div>

      <div className="flex gap-3">
        <div className="flex-1">
          <label className={labelCls}>Contact name</label>
          <input
            className={inputCls}
            value={value.contact_name}
            onChange={(e) => set({ contact_name: e.target.value })}
            placeholder="Primary contact"
          />
        </div>
        <div className="flex-1">
          <label className={labelCls}>Tax / registration ID</label>
          <input
            className={inputCls}
            value={value.tax_id}
            onChange={(e) => set({ tax_id: e.target.value })}
            placeholder="e.g. VAT / EIN"
          />
        </div>
      </div>

      <div className="flex gap-3">
        <div className="flex-1">
          <label className={labelCls}>Email</label>
          <input
            className={inputCls}
            value={value.email}
            onChange={(e) => set({ email: e.target.value })}
            placeholder="billing@vendor.com"
          />
        </div>
        <div className="flex-1">
          <label className={labelCls}>Phone</label>
          <input
            className={inputCls}
            value={value.phone}
            onChange={(e) => set({ phone: e.target.value })}
            placeholder="+1 555 000 0000"
          />
        </div>
      </div>

      <div>
        <label className={labelCls}>Website</label>
        <input
          className={inputCls}
          value={value.website}
          onChange={(e) => set({ website: e.target.value })}
          placeholder="https://vendor.com"
        />
      </div>

      <div>
        <label className={labelCls}>Address</label>
        <input
          className={inputCls}
          value={value.address_line}
          onChange={(e) => set({ address_line: e.target.value })}
          placeholder="Street address"
        />
      </div>

      <div className="flex gap-3">
        <div className="flex-1">
          <label className={labelCls}>City</label>
          <input
            className={inputCls}
            value={value.city}
            onChange={(e) => set({ city: e.target.value })}
          />
        </div>
        <div className="w-32 shrink-0">
          <label className={labelCls}>Postal code</label>
          <input
            className={inputCls}
            value={value.postal_code}
            onChange={(e) => set({ postal_code: e.target.value })}
          />
        </div>
        <div className="flex-1">
          <label className={labelCls}>Country</label>
          <input
            className={inputCls}
            value={value.country}
            onChange={(e) => set({ country: e.target.value })}
          />
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

      <label className="flex items-start gap-2">
        <input
          type="checkbox"
          className="mt-0.5"
          checked={value.registered}
          onChange={(e) => set({ registered: e.target.checked })}
        />
        <span className="text-sm">
          <span className="font-medium text-gray-700">Registered vendor</span>
          <span className="block text-xs text-gray-500">
            Mark vendors that have completed formal registration. Shown when picking a vendor on
            quotations and recommendations.
          </span>
        </span>
      </label>
    </div>
  );
}
