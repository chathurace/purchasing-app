import { Link } from "react-router-dom";
import { StatusBadge } from "./StatusBadge";
import { EntityStatusBadge } from "./EntityStatusBadge";
import type { CaseRecord } from "../lib/caseGraph";
import type { ContractStatus, InvoiceStatus, PRStatus, QuotationStatus } from "../types/api";

// RecordRow renders one clickable reference to a case record, with its status
// badge. Records on the winning path (selected quotation / signed contract) are
// tinted so the active branch stands out among siblings.
export function RecordRow({ record }: { record: CaseRecord }) {
  return (
    <div
      className={`flex items-center justify-between gap-3 ${
        record.highlight ? "-mx-2 rounded bg-green-50 px-2 py-1 ring-1 ring-green-200" : ""
      }`}
    >
      <div className="min-w-0">
        <Link to={record.to} className="font-medium text-indigo-600 hover:underline">
          {record.ref}
        </Link>
        {record.detail ? <span className="ml-2 text-gray-500">{record.detail}</span> : null}
      </div>
      {record.status == null ? null : record.kind === "pr" ? (
        <StatusBadge status={record.status as PRStatus} />
      ) : (
        <EntityStatusBadge status={record.status as QuotationStatus | ContractStatus | InvoiceStatus} />
      )}
    </div>
  );
}

// RecordCard is the standard "rounded white card with a heading" used for the
// related-record sections. Pass `records` for the default list rendering, or
// `children` for custom content; `action` adds a right-aligned link (e.g. an
// "+ Add" launcher).
export function RecordCard({
  title,
  action,
  actionProminent = false,
  records,
  empty = "None.",
  emptyCta,
  children,
}: {
  title: string;
  action?: { to: string; label: string };
  // When true, render the action as a solid button and (if there are no
  // records) surface a clickable dashed-border CTA in place of the empty text,
  // for steps that are the natural next action in the flow.
  actionProminent?: boolean;
  records?: CaseRecord[];
  empty?: string;
  emptyCta?: { title: string; subtitle: string };
  children?: React.ReactNode;
}) {
  const noRecords = !records || records.length === 0;
  return (
    <div className="app-card mt-6 p-6">
      <div className="mb-3 flex items-center justify-between">
        <h2 className="font-semibold text-slate-900">{title}</h2>
        {action ? (
          actionProminent ? (
            <Link
              to={action.to}
              className="inline-flex items-center gap-1.5 rounded-md bg-indigo-600 px-4 py-2 text-sm font-semibold text-white shadow-sm transition hover:bg-indigo-700"
            >
              <span className="text-base leading-none">+</span> {action.label.replace(/^\+\s*/, "")}
            </Link>
          ) : (
            <Link to={action.to} className="text-sm text-indigo-600">
              {action.label}
            </Link>
          )
        ) : null}
      </div>
      {children ??
        (!noRecords ? (
          <div className="space-y-1.5 text-sm">
            {records!.map((r) => (
              <RecordRow key={`${r.kind}-${r.id}`} record={r} />
            ))}
          </div>
        ) : actionProminent && action && emptyCta ? (
          <Link
            to={action.to}
            className="block rounded-lg border-2 border-dashed border-indigo-300 bg-indigo-50/60 p-5 text-center transition hover:border-indigo-400 hover:bg-indigo-50"
          >
            <p className="text-sm font-semibold text-indigo-900">{emptyCta.title}</p>
            <p className="mt-0.5 text-xs text-indigo-700/80">{emptyCta.subtitle}</p>
          </Link>
        ) : (
          <p className="text-sm text-gray-400">{empty}</p>
        ))}
    </div>
  );
}
