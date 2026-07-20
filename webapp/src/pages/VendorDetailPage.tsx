import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useVendor, useVendorUsage } from "../hooks/useVendors";
import { useCanManageVendors } from "../hooks/useCanManageVendors";
import { updateVendor } from "../api/vendors";
import { ApiError } from "../api/client";
import { VendorFields } from "../components/VendorFields";
import { vendorRef, type Vendor, type VendorInput } from "../types/api";

const labelCls = "font-medium text-gray-500";

function toInput(v: Vendor): VendorInput {
  return {
    name: v.name,
    contact_name: v.contact_name,
    email: v.email,
    phone: v.phone,
    notes: v.notes,
    is_active: v.is_active,
    tax_id: v.tax_id,
    address_line: v.address_line,
    city: v.city,
    postal_code: v.postal_code,
    country: v.country,
    website: v.website,
    registered: v.registered,
  };
}

export function VendorDetailPage() {
  const { id } = useParams();
  const vendorId = Number(id);
  const canManage = useCanManageVendors();
  const qc = useQueryClient();
  const { data: vendor, isLoading, error } = useVendor(vendorId);
  const { data: usage } = useVendorUsage(vendorId);

  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState<VendorInput | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ["vendors"] });
    qc.invalidateQueries({ queryKey: ["vendors", vendorId] });
  };

  const save = useMutation({
    mutationFn: (input: VendorInput) => updateVendor(vendorId, { ...input, name: input.name.trim() }),
    onSuccess: () => {
      invalidate();
      setEditing(false);
      setActionError(null);
    },
    onError: (e) => setActionError(e instanceof ApiError ? e.message : "Failed to save"),
  });

  const toggleActive = useMutation({
    mutationFn: () =>
      updateVendor(vendorId, { ...toInput(vendor!), is_active: !vendor!.is_active }),
    onSuccess: invalidate,
    onError: (e) => setActionError(e instanceof ApiError ? e.message : "Failed to update status"),
  });

  if (!canManage) {
    return (
      <div className="rounded border border-dashed bg-white p-8 text-center text-gray-500">
        You need the admin or procurement_admin role to manage vendors.
      </div>
    );
  }
  if (isLoading) return <p className="text-gray-500">Loading…</p>;
  if (error || !vendor) return <p className="text-red-600">Failed to load vendor.</p>;

  const startEdit = () => {
    setDraft(toInput(vendor));
    setEditing(true);
    setActionError(null);
  };

  return (
    <div className="mx-auto max-w-2xl">
      <div className="mb-4 flex items-center gap-2 text-sm text-gray-500">
        <Link to="/vendors" className="text-indigo-600">
          Vendors
        </Link>
        <span>/</span>
        <span>{vendorRef(vendor.id)}</span>
      </div>

      <div className="mb-4 flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold text-gray-900">{vendor.name}</h1>
          <div className="mt-1 flex items-center gap-2">
            {vendor.is_active ? (
              <span className="rounded bg-green-100 px-2 py-0.5 text-xs font-medium text-green-700">
                Active
              </span>
            ) : (
              <span className="rounded bg-red-100 px-2 py-0.5 text-xs font-medium text-red-700">
                Inactive
              </span>
            )}
            {vendor.registered && (
              <span className="rounded bg-indigo-100 px-2 py-0.5 text-xs font-medium text-indigo-700">
                Registered
              </span>
            )}
          </div>
        </div>
        {!editing && (
          <div className="flex gap-2">
            <button
              onClick={startEdit}
              className="rounded border px-3 py-2 text-sm text-gray-700 hover:bg-gray-50"
            >
              Edit details
            </button>
            <button
              onClick={() => toggleActive.mutate()}
              disabled={toggleActive.isPending}
              className="rounded border px-3 py-2 text-sm text-gray-700 hover:bg-gray-50 disabled:opacity-50"
            >
              {vendor.is_active ? "Deactivate" : "Reactivate"}
            </button>
          </div>
        )}
      </div>

      {actionError && <p className="mb-4 text-sm text-red-600">{actionError}</p>}

      <div className="rounded border bg-white p-6">
        {editing && draft ? (
          <div className="space-y-4">
            <VendorFields value={draft} onChange={setDraft} />
            <div className="flex gap-2">
              <button
                onClick={() => save.mutate(draft)}
                disabled={save.isPending || draft.name.trim() === ""}
                className="rounded bg-indigo-600 px-4 py-2 text-sm font-medium text-white hover:bg-indigo-700 disabled:opacity-50"
              >
                {save.isPending ? "Saving…" : "Save changes"}
              </button>
              <button
                onClick={() => {
                  setEditing(false);
                  setActionError(null);
                }}
                className="rounded border px-4 py-2 text-sm text-gray-700 hover:bg-gray-50"
              >
                Cancel
              </button>
            </div>
          </div>
        ) : (
          <dl className="grid grid-cols-2 gap-x-6 gap-y-4 text-sm">
            <Field label="Contact name" value={vendor.contact_name} />
            <Field label="Tax / registration ID" value={vendor.tax_id} />
            <Field label="Email" value={vendor.email} />
            <Field label="Phone" value={vendor.phone} />
            <Field label="Website" value={vendor.website} />
            <Field label="Address" value={vendor.address_line} />
            <Field
              label="City / postal / country"
              value={[vendor.city, vendor.postal_code, vendor.country].filter(Boolean).join(", ")}
            />
            <div className="col-span-2">
              <dt className={labelCls}>Notes</dt>
              <dd className="mt-0.5 whitespace-pre-wrap text-gray-900">{vendor.notes || "—"}</dd>
            </div>
          </dl>
        )}
      </div>

      {/* Where used */}
      <div className="mt-6 rounded border bg-white p-6">
        <h2 className="mb-3 font-medium text-gray-900">Where used</h2>
        {!usage ? (
          <p className="text-sm text-gray-400">Loading…</p>
        ) : (
          <div className="grid grid-cols-4 gap-3 text-center">
            <UsageStat to={`/quotations?vendor=${vendor.id}`} label="Quotations" count={usage.quotations} />
            <UsageStat to={`/contracts?vendor=${vendor.id}`} label="Contracts" count={usage.contracts} />
            <UsageStat to={`/grns?vendor=${vendor.id}`} label="GRNs" count={usage.grns} />
            <UsageStat to={`/invoices?vendor=${vendor.id}`} label="Invoices" count={usage.invoices} />
          </div>
        )}
        <p className="mt-3 text-xs text-gray-400">
          Vendors referenced by these records cannot be deleted — deactivate instead.
        </p>
      </div>
    </div>
  );
}

function Field({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className={labelCls}>{label}</dt>
      <dd className="mt-0.5 text-gray-900">{value || "—"}</dd>
    </div>
  );
}

function UsageStat({ to, label, count }: { to: string; label: string; count: number }) {
  return (
    <Link to={to} className="rounded border p-3 hover:bg-gray-50">
      <div className="text-2xl font-semibold text-gray-900">{count}</div>
      <div className="text-xs text-gray-500">{label}</div>
    </Link>
  );
}
