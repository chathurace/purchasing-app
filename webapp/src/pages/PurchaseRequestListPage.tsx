import { Link } from "react-router-dom";
import { usePurchaseRequests } from "../hooks/usePurchaseRequests";
import { useMe } from "../hooks/useMe";
import { StatusBadge } from "../components/StatusBadge";
import { prReference } from "../types/api";

export function PurchaseRequestListPage() {
  const { data, isLoading, error } = usePurchaseRequests();
  const { data: me } = useMe();

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight text-slate-900">Purchase requests</h1>
          <p className="mt-1 text-sm text-slate-500">Track and manage your purchasing requests.</p>
        </div>
        <Link to="/requests/new" className="btn-primary">
          <span className="text-base leading-none">+</span> New request
        </Link>
      </div>

      {isLoading && <p className="text-slate-500">Loading…</p>}
      {error && <p className="text-red-600">Failed to load requests.</p>}

      {data && data.length === 0 && (
        <div className="app-card border-dashed p-12 text-center">
          <p className="text-sm font-medium text-slate-700">No purchase requests yet</p>
          <p className="mt-1 text-sm text-slate-500">Create your first one to get started.</p>
          <Link to="/requests/new" className="btn-primary mt-4">
            <span className="text-base leading-none">+</span> New request
          </Link>
        </div>
      )}

      {data && data.length > 0 && (
        <div className="app-card overflow-hidden">
          <table className="w-full text-sm">
            <thead className="border-b border-slate-200 bg-slate-50/70 text-left text-xs font-medium uppercase tracking-wide text-slate-500">
              <tr>
                <th className="px-4 py-3">Ref</th>
                <th className="px-4 py-3">Title</th>
                <th className="px-4 py-3">Status</th>
                <th className="px-4 py-3">Assignee</th>
                <th className="px-4 py-3">Approvals</th>
                <th className="px-4 py-3">Created</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100">
              {data.map((pr) => (
                <tr key={pr.id} className="transition-colors hover:bg-slate-50/70">
                  <td className="px-4 py-3">
                    <Link to={`/requests/${pr.id}`} className="font-medium text-indigo-600 hover:text-indigo-700 hover:underline">
                      {prReference(pr)}
                    </Link>
                  </td>
                  <td className="px-4 py-3 text-slate-700">{pr.title || <span className="text-slate-400">—</span>}</td>
                  <td className="px-4 py-3">
                    <StatusBadge status={pr.status} />
                  </td>
                  <td className="px-4 py-3">
                    {me && pr.assignee_id === me.id ? (
                      <span className="badge bg-indigo-50 text-indigo-700 ring-indigo-600/20">
                        <span className="h-1.5 w-1.5 rounded-full bg-indigo-500" />
                        Assigned to you
                      </span>
                    ) : pr.assignee ? (
                      <span className="text-xs text-slate-600">{pr.assignee.name || pr.assignee.email}</span>
                    ) : pr.team_lead_status === "approved" ? (
                      <span className="badge bg-amber-50 text-amber-700 ring-amber-600/20">
                        <span className="h-1.5 w-1.5 rounded-full bg-amber-500" />
                        Unassigned
                      </span>
                    ) : (
                      <span className="text-slate-400">—</span>
                    )}
                  </td>
                  <td className="px-4 py-3">
                    {pr.my_approval_status === "pending" ? (
                      <span className="badge bg-amber-50 text-amber-700 ring-amber-600/20">
                        <span className="h-1.5 w-1.5 rounded-full bg-amber-500" />
                        Awaiting you
                      </span>
                    ) : pr.approvals_total > 0 ? (
                      <span className="text-xs text-slate-500">
                        {pr.approvals_approved}/{pr.approvals_total} approved
                      </span>
                    ) : (
                      <span className="text-slate-400">—</span>
                    )}
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
