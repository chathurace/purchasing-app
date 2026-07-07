import type { ContractStatus, InvoiceStatus, QuotationStatus } from "../types/api";

type AnyStatus = QuotationStatus | ContractStatus | InvoiceStatus;

const LABELS: Record<AnyStatus, string> = {
  // Quotation
  received: "Received",
  under_evaluation: "Under evaluation",
  selected: "Selected",
  // Contract
  draft: "Draft",
  approved: "Approved",
  signed: "Signed",
  // Invoice (received/approved shared with above); paid is invoice-only
  paid: "Paid",
  // shared: rejected
  rejected: "Rejected",
};

const CLASSES: Record<AnyStatus, string> = {
  received: "bg-blue-50 text-blue-700 ring-blue-600/20",
  under_evaluation: "bg-amber-50 text-amber-700 ring-amber-600/20",
  selected: "bg-emerald-50 text-emerald-700 ring-emerald-600/20",
  draft: "bg-slate-100 text-slate-600 ring-slate-500/20",
  approved: "bg-indigo-50 text-indigo-700 ring-indigo-600/20",
  signed: "bg-emerald-50 text-emerald-700 ring-emerald-600/20",
  paid: "bg-emerald-50 text-emerald-700 ring-emerald-600/20",
  rejected: "bg-red-50 text-red-700 ring-red-600/20",
};

const DOTS: Record<AnyStatus, string> = {
  received: "bg-blue-500",
  under_evaluation: "bg-amber-500",
  selected: "bg-emerald-500",
  draft: "bg-slate-400",
  approved: "bg-indigo-500",
  signed: "bg-emerald-500",
  paid: "bg-emerald-500",
  rejected: "bg-red-500",
};

export function EntityStatusBadge({ status }: { status: AnyStatus }) {
  return (
    <span className={`badge ${CLASSES[status] ?? "bg-slate-100 text-slate-600 ring-slate-500/20"}`}>
      <span className={`h-1.5 w-1.5 rounded-full ${DOTS[status] ?? "bg-slate-400"}`} />
      {LABELS[status] ?? status}
    </span>
  );
}
