import { Link, useParams } from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import { useContract } from "../hooks/useContracts";
import { useProcurementAccess } from "../hooks/useProcurementAccess";
import { EntityStatusBadge } from "../components/EntityStatusBadge";
import { ContractContent } from "../components/ContractContent";
import { RelatedDocuments } from "../components/RelatedDocuments";
import { ChainStepper } from "../components/ChainStepper";
import { DirectParentCard } from "../components/CaseSections";
import { ContractFulfillment } from "../components/ContractFulfillment";
import { conRef, formatMoney } from "../types/api";

export function ContractDetailPage() {
  const { id } = useParams();
  const conId = Number(id);
  const qc = useQueryClient();
  const { data: c, isLoading, error } = useContract(conId);
  const procurement = useProcurementAccess();

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ["contracts", conId] });
    qc.invalidateQueries({ queryKey: ["contracts"] });
    qc.invalidateQueries({ queryKey: ["purchase-requests"] });
  };

  if (isLoading) return <p className="text-slate-500">Loading…</p>;
  if (error || !c) return <p className="text-red-600">Failed to load contract.</p>;

  return (
    <div className="mx-auto max-w-2xl">
      <div className="mb-4 flex items-center gap-2 text-sm text-slate-400">
        <Link to="/contracts" className="text-indigo-600 hover:underline">
          Contracts
        </Link>
        <span>/</span>
        <span className="text-slate-500">{conRef(c.id)}</span>
      </div>

      <ChainStepper prId={c.purchase_request_id} current={{ kind: "contract", id: c.id }} />

      <div className="mb-4">
        <h1 className="text-2xl font-semibold tracking-tight text-slate-900">{c.title || conRef(c.id)}</h1>
        <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-sm text-slate-500">
          <EntityStatusBadge status={c.status} />
          <span>{c.vendor?.name ?? `Vendor #${c.vendor_id}`}</span>
          <span aria-hidden>·</span>
          <span>{formatMoney(c.total_amount, c.currency)}</span>
        </div>
      </div>

      <div className="app-card p-6">
        <h2 className="mb-4 font-semibold text-slate-900">Contract</h2>
        <ContractContent contract={c} canEdit={procurement} invalidate={invalidate} />
      </div>

      <DirectParentCard prId={c.purchase_request_id} current={{ kind: "contract", id: c.id }} />

      {/* Fulfillment: GRNs and invoices (once signed) — procurement-only; approvers
          get a read-only view of the contract itself without fulfillment. */}
      {procurement && <ContractFulfillment contract={c} />}

      <RelatedDocuments prId={c.purchase_request_id} current={{ kind: "contract", id: c.id }} />
    </div>
  );
}
