import { Link, useSearchParams } from "react-router-dom";
import { useQuotations } from "../hooks/useQuotations";
import { EntityStatusBadge } from "../components/EntityStatusBadge";
import { VendorFilterNotice } from "../components/VendorFilterNotice";
import { formatMoney, quoRef } from "../types/api";

export function QuotationListPage() {
  const { data, isLoading, error } = useQuotations();
  const [params] = useSearchParams();
  const vendorFilter = Number(params.get("vendor")) || 0;
  const rows = vendorFilter ? (data ?? []).filter((q) => q.vendor_id === vendorFilter) : data ?? [];
  const vendorName = rows.find((q) => q.vendor)?.vendor?.name;

  return (
    <div>
      <h1 className="mb-4 text-xl font-semibold text-gray-900">Quotations</h1>

      {vendorFilter > 0 && (
        <VendorFilterNotice vendorId={vendorFilter} vendorName={vendorName} basePath="/quotations" />
      )}

      {isLoading && <p className="text-gray-500">Loading…</p>}
      {error && <p className="text-red-600">Failed to load quotations.</p>}

      {data && rows.length === 0 && (
        <div className="rounded border border-dashed bg-white p-8 text-center text-gray-500">
          {vendorFilter > 0 ? "No quotations for this vendor." : "No quotations yet. Open a purchase request to add one."}
        </div>
      )}

      {data && rows.length > 0 && (
        <div className="overflow-hidden rounded border bg-white">
          <table className="w-full text-sm">
            <thead className="border-b bg-gray-50 text-left text-gray-500">
              <tr>
                <th className="px-4 py-2 font-medium">Ref</th>
                <th className="px-4 py-2 font-medium">Vendor</th>
                <th className="px-4 py-2 font-medium">Amount</th>
                <th className="px-4 py-2 font-medium">Request</th>
                <th className="px-4 py-2 font-medium">Status</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((q) => (
                <tr key={q.id} className="border-b last:border-0 hover:bg-gray-50">
                  <td className="px-4 py-2">
                    <Link to={`/quotations/${q.id}`} className="font-medium text-indigo-600">
                      {quoRef(q.id)}
                    </Link>
                  </td>
                  <td className="px-4 py-2 text-gray-700">
                    {q.vendor?.name ?? `Vendor #${q.vendor_id}`}
                  </td>
                  <td className="px-4 py-2 text-gray-700">{formatMoney(q.total_amount, q.currency)}</td>
                  <td className="px-4 py-2">
                    <Link to={`/requests/${q.purchase_request_id}`} className="text-indigo-600">
                      {`#${q.purchase_request_id}`}
                    </Link>
                  </td>
                  <td className="px-4 py-2">
                    <EntityStatusBadge status={q.status} />
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
