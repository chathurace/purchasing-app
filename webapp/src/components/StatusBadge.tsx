import type { PRStatus } from "../types/api";

const LABELS: Record<PRStatus, string> = {
  submitted: "Submitted",
  under_review: "Under review",
  vendor_selected: "Vendor selected",
  contract_prepared: "Contract prepared",
  order_signed: "Order signed",
  completed: "Completed",
  rejected: "Rejected",
  cancelled: "Cancelled",
};

// Each status carries its pill colors plus a matching dot color.
const CLASSES: Record<PRStatus, string> = {
  submitted: "bg-blue-50 text-blue-700 ring-blue-600/20",
  under_review: "bg-amber-50 text-amber-700 ring-amber-600/20",
  vendor_selected: "bg-indigo-50 text-indigo-700 ring-indigo-600/20",
  contract_prepared: "bg-violet-50 text-violet-700 ring-violet-600/20",
  order_signed: "bg-emerald-50 text-emerald-700 ring-emerald-600/20",
  completed: "bg-emerald-50 text-emerald-700 ring-emerald-600/20",
  rejected: "bg-red-50 text-red-700 ring-red-600/20",
  cancelled: "bg-slate-100 text-slate-600 ring-slate-500/20",
};

const DOTS: Record<PRStatus, string> = {
  submitted: "bg-blue-500",
  under_review: "bg-amber-500",
  vendor_selected: "bg-indigo-500",
  contract_prepared: "bg-violet-500",
  order_signed: "bg-emerald-500",
  completed: "bg-emerald-500",
  rejected: "bg-red-500",
  cancelled: "bg-slate-400",
};

export function StatusBadge({ status }: { status: PRStatus }) {
  return (
    <span className={`badge ${CLASSES[status] ?? CLASSES.cancelled}`}>
      <span className={`h-1.5 w-1.5 rounded-full ${DOTS[status] ?? DOTS.cancelled}`} />
      {LABELS[status] ?? status}
    </span>
  );
}
