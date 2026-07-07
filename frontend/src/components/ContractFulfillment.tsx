import { Link } from "react-router-dom";
import { useGRNsForContract } from "../hooks/useGrns";
import { useInvoicesForContract } from "../hooks/useInvoices";
import { EntityStatusBadge } from "./EntityStatusBadge";
import { formatMoney, grnRef, invRef } from "../types/api";
import type { Contract } from "../types/api";

// ContractFulfillment shows the goods-received notes and invoices recorded
// against a signed contract, plus the invoiced-vs-contract total summary with an
// over-billing warning. Only meaningful once the contract is signed.
export function ContractFulfillment({ contract }: { contract: Contract }) {
  const signed = contract.status === "signed";
  const { data: grns } = useGRNsForContract(contract.id, signed);
  const { data: invoices } = useInvoicesForContract(contract.id, signed);

  if (!signed) {
    return (
      <div className="mt-6 rounded border bg-white p-6">
        <h2 className="mb-1 font-medium text-gray-900">Goods received &amp; invoices</h2>
        <p className="text-sm text-gray-400">
          Available once the contract is signed. Sign the order above to start recording GRNs and invoices.
        </p>
      </div>
    );
  }

  const invoicedTotal = contract.invoiced_total ?? 0;
  const remaining = contract.total_amount - invoicedTotal;
  const overBilled = invoicedTotal > contract.total_amount;

  return (
    <>
      {/* Goods received notes */}
      <div className="mt-6 rounded border bg-white p-6">
        <div className="mb-3 flex items-center justify-between">
          <h2 className="font-medium text-gray-900">Goods received (GRNs)</h2>
          <Link
            to={`/contracts/${contract.id}/grns/new`}
            className="inline-flex items-center gap-1.5 rounded-md bg-indigo-600 px-4 py-2 text-sm font-semibold text-white shadow-sm transition hover:bg-indigo-700"
          >
            <span className="text-base leading-none">+</span> New GRN
          </Link>
        </div>
        {!grns ? (
          <p className="text-sm text-gray-400">Loading…</p>
        ) : grns.length === 0 ? (
          <Link
            to={`/contracts/${contract.id}/grns/new`}
            className="block rounded-lg border-2 border-dashed border-indigo-300 bg-indigo-50/60 p-5 text-center transition hover:border-indigo-400 hover:bg-indigo-50"
          >
            <p className="text-sm font-semibold text-indigo-900">Record the first GRN</p>
            <p className="mt-0.5 text-xs text-indigo-700/80">
              Log goods received against this signed contract.
            </p>
          </Link>
        ) : (
          <ul className="divide-y">
            {grns.map((g) => (
              <li key={g.id} className="flex items-center justify-between py-2 text-sm">
                <Link to={`/grns/${g.id}`} className="text-indigo-600 hover:underline">
                  {grnRef(g.id)}
                </Link>
                <span className="text-gray-500">
                  received {g.received_date}
                  {g.received_by ? ` · ${g.received_by}` : ""}
                </span>
              </li>
            ))}
          </ul>
        )}
      </div>

      {/* Invoices */}
      <div className="mt-6 rounded border bg-white p-6">
        <div className="mb-3 flex items-center justify-between">
          <h2 className="font-medium text-gray-900">Invoices</h2>
          <Link
            to={`/contracts/${contract.id}/invoices/new`}
            className="inline-flex items-center gap-1.5 rounded-md bg-indigo-600 px-4 py-2 text-sm font-semibold text-white shadow-sm transition hover:bg-indigo-700"
          >
            <span className="text-base leading-none">+</span> New invoice
          </Link>
        </div>

        {/* Invoiced-vs-contract summary (over-billing warning) */}
        <div
          className={`mb-3 rounded p-3 text-sm ${
            overBilled ? "bg-red-50 text-red-700" : "bg-gray-50 text-gray-600"
          }`}
        >
          Invoiced {formatMoney(invoicedTotal, contract.currency)} of{" "}
          {formatMoney(contract.total_amount, contract.currency)}
          {overBilled ? (
            <span className="font-medium">
              {" "}
              — over contract by {formatMoney(invoicedTotal - contract.total_amount, contract.currency)}
            </span>
          ) : (
            <span> · {formatMoney(remaining, contract.currency)} remaining</span>
          )}
        </div>

        {!invoices ? (
          <p className="text-sm text-gray-400">Loading…</p>
        ) : invoices.length === 0 ? (
          <Link
            to={`/contracts/${contract.id}/invoices/new`}
            className="block rounded-lg border-2 border-dashed border-indigo-300 bg-indigo-50/60 p-5 text-center transition hover:border-indigo-400 hover:bg-indigo-50"
          >
            <p className="text-sm font-semibold text-indigo-900">Record the first invoice</p>
            <p className="mt-0.5 text-xs text-indigo-700/80">
              Enter a vendor invoice billed against this contract.
            </p>
          </Link>
        ) : (
          <ul className="divide-y">
            {invoices.map((inv) => (
              <li key={inv.id} className="flex items-center justify-between py-2 text-sm">
                <Link to={`/invoices/${inv.id}`} className="text-indigo-600 hover:underline">
                  {inv.vendor_invoice_no ? `${invRef(inv.id)} · ${inv.vendor_invoice_no}` : invRef(inv.id)}
                </Link>
                <span className="flex items-center gap-2 text-gray-500">
                  {formatMoney(inv.total_amount, inv.currency)}
                  <EntityStatusBadge status={inv.status} />
                </span>
              </li>
            ))}
          </ul>
        )}
      </div>
    </>
  );
}
