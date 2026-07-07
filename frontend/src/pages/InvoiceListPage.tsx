import { Link, useSearchParams } from "react-router-dom";
import { useInvoices } from "../hooks/useInvoices";
import { EntityStatusBadge } from "../components/EntityStatusBadge";
import { VendorFilterNotice } from "../components/VendorFilterNotice";
import { conRef, formatMoney, invRef } from "../types/api";

export function InvoiceListPage() {
  const { data, isLoading, error } = useInvoices();
  const [params] = useSearchParams();
  const vendorFilter = Number(params.get("vendor")) || 0;
  const rows = vendorFilter ? (data ?? []).filter((inv) => inv.vendor_id === vendorFilter) : data ?? [];
  const vendorName = rows.find((inv) => inv.vendor)?.vendor?.name;

  return (
    <div>
      <h1 className="mb-4 text-xl font-semibold text-gray-900">Invoices</h1>

      {vendorFilter > 0 && (
        <VendorFilterNotice vendorId={vendorFilter} vendorName={vendorName} basePath="/invoices" />
      )}

      {isLoading && <p className="text-gray-500">Loading…</p>}
      {error && <p className="text-red-600">Failed to load invoices.</p>}

      {data && rows.length === 0 && (
        <div className="rounded border border-dashed bg-white p-8 text-center text-gray-500">
          {vendorFilter > 0 ? "No invoices for this vendor." : "No invoices yet. Record one from a signed contract."}
        </div>
      )}

      {data && rows.length > 0 && (
        <div className="overflow-hidden rounded border bg-white">
          <table className="w-full text-sm">
            <thead className="border-b bg-gray-50 text-left text-gray-500">
              <tr>
                <th className="px-4 py-2 font-medium">Ref</th>
                <th className="px-4 py-2 font-medium">Vendor invoice no.</th>
                <th className="px-4 py-2 font-medium">Vendor</th>
                <th className="px-4 py-2 font-medium">Amount</th>
                <th className="px-4 py-2 font-medium">Invoice date</th>
                <th className="px-4 py-2 font-medium">Contract</th>
                <th className="px-4 py-2 font-medium">Status</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((inv) => (
                <tr key={inv.id} className="border-b last:border-0 hover:bg-gray-50">
                  <td className="px-4 py-2">
                    <Link to={`/invoices/${inv.id}`} className="font-medium text-indigo-600">
                      {invRef(inv.id)}
                    </Link>
                  </td>
                  <td className="px-4 py-2 text-gray-700">
                    {inv.vendor_invoice_no || <span className="text-gray-400">—</span>}
                  </td>
                  <td className="px-4 py-2 text-gray-700">{inv.vendor?.name ?? `Vendor #${inv.vendor_id}`}</td>
                  <td className="px-4 py-2 text-gray-700">{formatMoney(inv.total_amount, inv.currency)}</td>
                  <td className="px-4 py-2 text-gray-700">{inv.invoice_date}</td>
                  <td className="px-4 py-2">
                    <Link to={`/contracts/${inv.contract_id}`} className="text-indigo-600">
                      {conRef(inv.contract_id)}
                    </Link>
                  </td>
                  <td className="px-4 py-2">
                    <EntityStatusBadge status={inv.status} />
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
