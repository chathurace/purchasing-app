import { useState } from "react";
import { Link } from "react-router-dom";
import { useApprovalRequests } from "../hooks/usePurchaseRequests";
import { StatusBadge } from "../components/StatusBadge";
import { prReference, type ApprovalStatus } from "../types/api";

// MyApprovalBadge renders the caller's own state on a PR awaiting them.
function MyApprovalBadge({ state }: { state?: ApprovalStatus | null }) {
  if (state === "pending") {
    return (
      <span className="badge bg-amber-50 text-amber-700 ring-amber-600/20">
        <span className="h-1.5 w-1.5 rounded-full bg-amber-500" />
        Awaiting you
      </span>
    );
  }
  if (state === "approved") {
    return (
      <span className="badge bg-emerald-50 text-emerald-700 ring-emerald-600/20">
        <span className="h-1.5 w-1.5 rounded-full bg-emerald-500" />
        Approved
      </span>
    );
  }
  if (state === "rejected") {
    return (
      <span className="badge bg-red-50 text-red-700 ring-red-600/20">
        <span className="h-1.5 w-1.5 rounded-full bg-red-500" />
        Rejected
      </span>
    );
  }
  return <span className="text-slate-400">—</span>;
}

export function ApprovalsListPage() {
  const { data, isLoading, error } = useApprovalRequests();
  // Two independent filters. Pending is on by default; "Reviewed" covers the
  // acted-on rows (approved + rejected — recommendation cards can't be rejected,
  // so a rejected row is always a named-approval decision).
  const [showPending, setShowPending] = useState(true);
  const [showReviewed, setShowReviewed] = useState(false);

  const rows = (data ?? []).filter((pr) => {
    const state = pr.my_approval_state;
    if (state === "pending") return showPending;
    return showReviewed; // approved or rejected
  });

  return (
    <div>
      <div className="mb-6">
        <h1 className="text-2xl font-semibold tracking-tight text-slate-900">Approvals</h1>
        <p className="mt-1 text-sm text-slate-500">
          Purchase requests awaiting your decision as a budget, legal, or security approver.
        </p>
      </div>

      <div className="mb-4 flex items-center gap-5 text-sm">
        <label className="flex cursor-pointer items-center gap-2 text-slate-700">
          <input
            type="checkbox"
            className="h-4 w-4 rounded border-slate-300 text-indigo-600 focus:ring-indigo-500"
            checked={showPending}
            onChange={(e) => setShowPending(e.target.checked)}
          />
          Pending
        </label>
        <label className="flex cursor-pointer items-center gap-2 text-slate-700">
          <input
            type="checkbox"
            className="h-4 w-4 rounded border-slate-300 text-indigo-600 focus:ring-indigo-500"
            checked={showReviewed}
            onChange={(e) => setShowReviewed(e.target.checked)}
          />
          Approved
        </label>
      </div>

      {isLoading && <p className="text-slate-500">Loading…</p>}
      {error && <p className="text-red-600">Failed to load approvals.</p>}

      {data && rows.length === 0 && (
        <div className="app-card border-dashed p-12 text-center">
          <p className="text-sm font-medium text-slate-700">Nothing to show</p>
          <p className="mt-1 text-sm text-slate-500">
            {data.length === 0
              ? "No purchase requests are awaiting your approval."
              : "No requests match the selected filters."}
          </p>
        </div>
      )}

      {rows.length > 0 && (
        <div className="app-card overflow-hidden">
          <table className="w-full text-sm">
            <thead className="border-b border-slate-200 bg-slate-50/70 text-left text-xs font-medium uppercase tracking-wide text-slate-500">
              <tr>
                <th className="px-4 py-3">Ref</th>
                <th className="px-4 py-3">Title</th>
                <th className="px-4 py-3">Requester</th>
                <th className="px-4 py-3">Status</th>
                <th className="px-4 py-3">Your decision</th>
                <th className="px-4 py-3">Created</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100">
              {rows.map((pr) => (
                <tr key={pr.id} className="transition-colors hover:bg-slate-50/70">
                  <td className="px-4 py-3">
                    <Link
                      to={`/requests/${pr.id}`}
                      className="font-medium text-indigo-600 hover:text-indigo-700 hover:underline"
                    >
                      {prReference(pr)}
                    </Link>
                  </td>
                  <td className="px-4 py-3 text-slate-700">
                    {pr.title || <span className="text-slate-400">—</span>}
                  </td>
                  <td className="px-4 py-3 text-slate-500">
                    {pr.requester?.name || pr.requester?.email || "—"}
                  </td>
                  <td className="px-4 py-3">
                    <StatusBadge status={pr.status} />
                  </td>
                  <td className="px-4 py-3">
                    <MyApprovalBadge state={pr.my_approval_state} />
                  </td>
                  <td className="px-4 py-3 text-slate-500">
                    {new Date(pr.created_at).toLocaleDateString()}
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
