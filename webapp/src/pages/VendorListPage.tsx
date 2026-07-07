import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useVendors } from "../hooks/useVendors";
import { useCanManageVendors } from "../hooks/useCanManageVendors";
import { createVendor } from "../api/vendors";
import { ApiError } from "../api/client";
import { VendorFields, emptyVendor } from "../components/VendorFields";
import { vendorRef, type VendorInput } from "../types/api";

type StatusFilter = "active" | "inactive" | "all";

export function VendorListPage() {
  const canManage = useCanManageVendors();
  const { data: vendors, isLoading, error } = useVendors();
  const qc = useQueryClient();

  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("active");
  const [adding, setAdding] = useState(false);
  const [draft, setDraft] = useState<VendorInput>(emptyVendor);
  const [formError, setFormError] = useState<string | null>(null);

  const create = useMutation({
    mutationFn: () => createVendor({ ...draft, name: draft.name.trim() }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["vendors"] });
      setAdding(false);
      setDraft(emptyVendor);
      setFormError(null);
    },
    onError: (e) => setFormError(e instanceof ApiError ? e.message : "Failed to create vendor"),
  });

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return (vendors ?? [])
      .filter((v) =>
        statusFilter === "all" ? true : statusFilter === "active" ? v.is_active : !v.is_active,
      )
      .filter((v) =>
        q === ""
          ? true
          : v.name.toLowerCase().includes(q) ||
            v.contact_name.toLowerCase().includes(q) ||
            v.email.toLowerCase().includes(q),
      );
  }, [vendors, search, statusFilter]);

  if (!canManage) {
    return (
      <div className="rounded border border-dashed bg-white p-8 text-center text-gray-500">
        You need the admin or finance_admin role to manage vendors.
      </div>
    );
  }

  return (
    <div>
      <div className="mb-1 flex items-center justify-between">
        <h1 className="text-xl font-semibold text-gray-900">Vendors</h1>
        <button
          type="button"
          onClick={() => {
            setAdding((a) => !a);
            setFormError(null);
            setDraft(emptyVendor);
          }}
          className="rounded bg-indigo-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-indigo-700"
        >
          {adding ? "Cancel" : "+ New vendor"}
        </button>
      </div>
      <p className="mb-6 text-sm text-gray-500">
        Manage the vendor master used across quotations, contracts, GRNs and invoices. Deactivate a
        vendor to retire it without losing history — inactive vendors can no longer be selected on
        new records.
      </p>

      {adding && (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            create.mutate();
          }}
          className="mb-6 rounded border bg-white p-4"
        >
          <VendorFields value={draft} onChange={setDraft} />
          {formError && <p className="mt-2 text-sm text-red-600">{formError}</p>}
          <div className="mt-4">
            <button
              type="submit"
              disabled={create.isPending || draft.name.trim() === ""}
              className="rounded bg-indigo-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-indigo-700 disabled:opacity-50"
            >
              {create.isPending ? "Adding…" : "Add vendor"}
            </button>
          </div>
        </form>
      )}

      <div className="mb-4 flex flex-wrap items-center gap-3">
        <input
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search by name, contact or email…"
          className="w-72 rounded border px-3 py-1.5 text-sm"
        />
        <select
          value={statusFilter}
          onChange={(e) => setStatusFilter(e.target.value as StatusFilter)}
          className="rounded border px-2 py-1.5 text-sm text-gray-700"
        >
          <option value="active">Active</option>
          <option value="inactive">Inactive</option>
          <option value="all">All</option>
        </select>
      </div>

      {isLoading && <p className="text-gray-500">Loading…</p>}
      {error && <p className="text-red-600">Failed to load vendors.</p>}

      {vendors && (
        <div className="overflow-hidden rounded border bg-white">
          <table className="w-full text-sm">
            <thead className="border-b bg-gray-50 text-left text-gray-500">
              <tr>
                <th className="px-4 py-2 font-medium">Vendor</th>
                <th className="px-4 py-2 font-medium">Contact</th>
                <th className="px-4 py-2 font-medium">Status</th>
              </tr>
            </thead>
            <tbody>
              {filtered.length === 0 && (
                <tr>
                  <td colSpan={3} className="px-4 py-6 text-center text-gray-400">
                    No vendors match.
                  </td>
                </tr>
              )}
              {filtered.map((v) => (
                <tr key={v.id} className={`border-b last:border-0 ${v.is_active ? "" : "bg-gray-50"}`}>
                  <td className="px-4 py-3 align-top">
                    <Link to={`/vendors/${v.id}`} className="font-medium text-indigo-600">
                      {v.name}
                    </Link>
                    <div className="text-xs text-gray-400">{vendorRef(v.id)}</div>
                  </td>
                  <td className="px-4 py-3 align-top text-gray-700">
                    <div>{v.contact_name || "—"}</div>
                    {v.email && <div className="text-gray-500">{v.email}</div>}
                  </td>
                  <td className="px-4 py-3 align-top">
                    <div className="flex flex-wrap items-center gap-1.5">
                      {v.is_active ? (
                        <span className="rounded bg-green-100 px-2 py-0.5 text-xs font-medium text-green-700">
                          Active
                        </span>
                      ) : (
                        <span className="rounded bg-red-100 px-2 py-0.5 text-xs font-medium text-red-700">
                          Inactive
                        </span>
                      )}
                      {v.registered && (
                        <span className="rounded bg-indigo-100 px-2 py-0.5 text-xs font-medium text-indigo-700">
                          Registered
                        </span>
                      )}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
