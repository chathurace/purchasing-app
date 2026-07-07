import { Link, useSearchParams } from "react-router-dom";
import { useContracts } from "../hooks/useContracts";
import { EntityStatusBadge } from "../components/EntityStatusBadge";
import { VendorFilterNotice } from "../components/VendorFilterNotice";
import { conRef, formatMoney } from "../types/api";

export function ContractListPage() {
  const { data, isLoading, error } = useContracts();
  const [params] = useSearchParams();
  const vendorFilter = Number(params.get("vendor")) || 0;
  const rows = vendorFilter ? (data ?? []).filter((c) => c.vendor_id === vendorFilter) : data ?? [];
  const vendorName = rows.find((c) => c.vendor)?.vendor?.name;

  return (
    <div>
      <div className="mb-6">
        <h1 className="text-2xl font-semibold tracking-tight text-slate-900">Contracts</h1>
        <p className="mt-1 text-sm text-slate-500">Vendor contracts across all purchase requests.</p>
      </div>

      {vendorFilter > 0 && (
        <VendorFilterNotice vendorId={vendorFilter} vendorName={vendorName} basePath="/contracts" />
      )}

      {isLoading && <p className="text-slate-500">Loading…</p>}
      {error && <p className="text-red-600">Failed to load contracts.</p>}

      {data && rows.length === 0 && (
        <div className="app-card border-dashed p-12 text-center text-slate-500">
          {vendorFilter > 0 ? "No contracts for this vendor." : "No contracts yet. Create one from a quotation."}
        </div>
      )}

      {data && rows.length > 0 && (
        <div className="app-card overflow-hidden">
          <table className="w-full text-sm">
            <thead className="border-b border-slate-200 bg-slate-50/70 text-left text-xs font-medium uppercase tracking-wide text-slate-500">
              <tr>
                <th className="px-4 py-3">Ref</th>
                <th className="px-4 py-3">Title</th>
                <th className="px-4 py-3">Vendor</th>
                <th className="px-4 py-3">Amount</th>
                <th className="px-4 py-3">Request</th>
                <th className="px-4 py-3">Status</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100">
              {rows.map((c) => (
                <tr key={c.id} className="transition-colors hover:bg-slate-50/70">
                  <td className="px-4 py-3">
                    <Link to={`/contracts/${c.id}`} className="font-medium text-indigo-600 hover:text-indigo-700 hover:underline">
                      {conRef(c.id)}
                    </Link>
                  </td>
                  <td className="px-4 py-3 text-slate-700">
                    {c.title || <span className="text-slate-400">—</span>}
                  </td>
                  <td className="px-4 py-3 text-slate-700">
                    {c.vendor?.name ?? `Vendor #${c.vendor_id}`}
                  </td>
                  <td className="px-4 py-3 text-slate-700">{formatMoney(c.total_amount, c.currency)}</td>
                  <td className="px-4 py-3">
                    <Link to={`/requests/${c.purchase_request_id}`} className="text-indigo-600 hover:underline">
                      {`#${c.purchase_request_id}`}
                    </Link>
                  </td>
                  <td className="px-4 py-3">
                    <EntityStatusBadge status={c.status} />
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
