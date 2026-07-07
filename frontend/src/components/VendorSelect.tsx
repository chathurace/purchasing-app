import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useVendors } from "../hooks/useVendors";
import { createVendor } from "../api/vendors";
import { ApiError } from "../api/client";
import { emptyVendor } from "./VendorFields";
import type { VendorInput } from "../types/api";

const inputCls = "w-full rounded border px-3 py-2 text-sm focus:border-indigo-500 focus:outline-none";

interface Props {
  value: number; // selected vendor id (0 = none)
  onChange: (vendorId: number) => void;
}

export function VendorSelect({ value, onChange }: Props) {
  const qc = useQueryClient();
  const { data: vendors } = useVendors();
  const [adding, setAdding] = useState(false);
  const [draft, setDraft] = useState<VendorInput>(emptyVendor);
  const [error, setError] = useState<string | null>(null);

  const createMutation = useMutation({
    mutationFn: () => createVendor({ ...draft, name: draft.name.trim() }),
    onSuccess: (v) => {
      qc.invalidateQueries({ queryKey: ["vendors"] });
      onChange(v.id);
      setAdding(false);
      setDraft(emptyVendor);
      setError(null);
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : "Failed to create vendor"),
  });

  return (
    <div>
      <div className="flex items-center justify-between">
        <label className="block text-sm font-medium text-gray-700 mb-1">Vendor</label>
        <button
          type="button"
          className="text-sm text-indigo-600"
          onClick={() => setAdding((a) => !a)}
        >
          {adding ? "Cancel" : "+ New vendor"}
        </button>
      </div>

      {!adding && (
        <select
          className={inputCls}
          value={value || ""}
          onChange={(e) => onChange(Number(e.target.value))}
        >
          <option value="">Select a vendor…</option>
          {vendors
            // Only active vendors can be picked for new records, but keep the
            // currently selected vendor visible even if it was deactivated.
            ?.filter((v) => v.is_active || v.id === value)
            .map((v) => {
              const tags = [
                !v.is_active ? "inactive" : null,
                !v.registered ? "unregistered" : null,
              ].filter(Boolean);
              return (
                <option key={v.id} value={v.id}>
                  {v.name}
                  {tags.length ? ` (${tags.join(", ")})` : ""}
                </option>
              );
            })}
        </select>
      )}

      {adding && (
        <div className="space-y-2 rounded border bg-gray-50 p-3">
          <input
            className={inputCls}
            value={draft.name}
            onChange={(e) => setDraft({ ...draft, name: e.target.value })}
            placeholder="Vendor name *"
          />
          <div className="flex gap-2">
            <input
              className={inputCls}
              value={draft.contact_name}
              onChange={(e) => setDraft({ ...draft, contact_name: e.target.value })}
              placeholder="Contact name"
            />
            <input
              className={inputCls}
              value={draft.email}
              onChange={(e) => setDraft({ ...draft, email: e.target.value })}
              placeholder="Email"
            />
          </div>
          {error && <p className="text-sm text-red-600">{error}</p>}
          <button
            type="button"
            disabled={createMutation.isPending || draft.name.trim() === ""}
            onClick={() => createMutation.mutate()}
            className="rounded bg-indigo-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-indigo-700 disabled:opacity-50"
          >
            {createMutation.isPending ? "Adding…" : "Add vendor"}
          </button>
        </div>
      )}
    </div>
  );
}
