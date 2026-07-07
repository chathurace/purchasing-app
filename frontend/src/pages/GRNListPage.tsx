import { Link, useSearchParams } from "react-router-dom";
import { useGRNs } from "../hooks/useGrns";
import { VendorFilterNotice } from "../components/VendorFilterNotice";
import { conRef, grnRef } from "../types/api";

export function GRNListPage() {
  const { data, isLoading, error } = useGRNs();
  const [params] = useSearchParams();
  const vendorFilter = Number(params.get("vendor")) || 0;
  const rows = vendorFilter ? (data ?? []).filter((g) => g.vendor_id === vendorFilter) : data ?? [];
  const vendorName = rows.find((g) => g.vendor)?.vendor?.name;

  return (
    <div>
      <h1 className="mb-4 text-xl font-semibold text-gray-900">Goods received notes</h1>

      {vendorFilter > 0 && (
        <VendorFilterNotice vendorId={vendorFilter} vendorName={vendorName} basePath="/grns" />
      )}

      {isLoading && <p className="text-gray-500">Loading…</p>}
      {error && <p className="text-red-600">Failed to load GRNs.</p>}

      {data && rows.length === 0 && (
        <div className="rounded border border-dashed bg-white p-8 text-center text-gray-500">
          {vendorFilter > 0 ? "No GRNs for this vendor." : "No GRNs yet. Record one from a signed contract."}
        </div>
      )}

      {data && rows.length > 0 && (
        <div className="overflow-hidden rounded border bg-white">
          <table className="w-full text-sm">
            <thead className="border-b bg-gray-50 text-left text-gray-500">
              <tr>
                <th className="px-4 py-2 font-medium">Ref</th>
                <th className="px-4 py-2 font-medium">Vendor</th>
                <th className="px-4 py-2 font-medium">Received</th>
                <th className="px-4 py-2 font-medium">Contract</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((g) => (
                <tr key={g.id} className="border-b last:border-0 hover:bg-gray-50">
                  <td className="px-4 py-2">
                    <Link to={`/grns/${g.id}`} className="font-medium text-indigo-600">
                      {grnRef(g.id)}
                    </Link>
                  </td>
                  <td className="px-4 py-2 text-gray-700">{g.vendor?.name ?? `Vendor #${g.vendor_id}`}</td>
                  <td className="px-4 py-2 text-gray-700">{g.received_date}</td>
                  <td className="px-4 py-2">
                    <Link to={`/contracts/${g.contract_id}`} className="text-indigo-600">
                      {conRef(g.contract_id)}
                    </Link>
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
