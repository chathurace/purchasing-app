import { useState } from "react";
import { APPROVAL_STATUS_LABELS } from "../types/api";
import type { ApprovalStatus, PRApproval, UserSummary } from "../types/api";

interface Props {
  approvals: PRApproval[];
  meId: number | undefined;
  // canManage: the requester, while the request is editable — may remove an
  // approver or re-request a rejected approval.
  canManage: boolean;
  busy: boolean;
  onRemove: (approverId: number) => void;
  onRequestAgain: (approverId: number) => void;
  onDecide: (decision: "approve" | "reject", comment: string) => void;
}

const statusCls: Record<ApprovalStatus, string> = {
  pending: "bg-amber-100 text-amber-800",
  approved: "bg-green-100 text-green-800",
  rejected: "bg-red-100 text-red-800",
};

function approverLabel(u?: UserSummary | null, id?: number): string {
  if (!u) return id != null ? `#${id}` : "";
  return u.name ? `${u.name} (${u.email})` : u.email || `#${u.id}`;
}

export function ApprovalList({
  approvals,
  meId,
  canManage,
  busy,
  onRemove,
  onRequestAgain,
  onDecide,
}: Props) {
  const approvedCount = approvals.filter((a) => a.status === "approved").length;

  return (
    <div className="mt-6 rounded border bg-white p-6">
      <div className="mb-3 flex items-center justify-between">
        <h2 className="font-medium text-gray-900">Approvals</h2>
        {approvals.length > 0 && (
          <span className="text-xs text-gray-500">
            {approvedCount} of {approvals.length} approved
          </span>
        )}
      </div>

      {approvals.length === 0 ? (
        <p className="text-sm text-gray-400">No approvers.</p>
      ) : (
        <ul className="divide-y">
          {approvals.map((a) => (
            <ApprovalRow
              key={a.id}
              approval={a}
              isMine={meId != null && a.approver_id === meId}
              canManage={canManage}
              busy={busy}
              onRemove={() => onRemove(a.approver_id)}
              onRequestAgain={() => onRequestAgain(a.approver_id)}
              onDecide={onDecide}
            />
          ))}
        </ul>
      )}
    </div>
  );
}

function ApprovalRow({
  approval: a,
  isMine,
  canManage,
  busy,
  onRemove,
  onRequestAgain,
  onDecide,
}: {
  approval: PRApproval;
  isMine: boolean;
  canManage: boolean;
  busy: boolean;
  onRemove: () => void;
  onRequestAgain: () => void;
  onDecide: (decision: "approve" | "reject", comment: string) => void;
}) {
  const [deciding, setDeciding] = useState(false);
  const [comment, setComment] = useState("");
  const canDecide = isMine && a.status === "pending";

  return (
    <li className="py-3 text-sm">
      <div className="flex items-center justify-between">
        <span className="text-gray-800">
          {approverLabel(a.approver, a.approver_id)}
          {isMine && <span className="ml-2 text-xs text-indigo-500">(you)</span>}
        </span>
        <span className={`rounded-full px-2 py-0.5 text-xs font-medium ${statusCls[a.status]}`}>
          {APPROVAL_STATUS_LABELS[a.status]}
        </span>
      </div>

      {a.comment && (
        <p className="mt-1 whitespace-pre-wrap text-gray-600">
          <span className="text-gray-400">Comment: </span>
          {a.comment}
        </p>
      )}

      {/* The acting approver decides on their own pending row. */}
      {canDecide && !deciding && (
        <div className="mt-2 flex gap-2">
          <button
            type="button"
            disabled={busy}
            onClick={() => onDecide("approve", "")}
            className="rounded bg-green-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-green-700 disabled:opacity-50"
          >
            Approve
          </button>
          <button
            type="button"
            disabled={busy}
            onClick={() => setDeciding(true)}
            className="rounded border border-red-300 px-3 py-1.5 text-xs font-medium text-red-700 hover:bg-red-50 disabled:opacity-50"
          >
            Reject…
          </button>
        </div>
      )}

      {canDecide && deciding && (
        <div className="mt-2 space-y-2 rounded border border-red-200 bg-red-50 p-3">
          <textarea
            className="w-full rounded border px-3 py-2 text-sm focus:border-indigo-500 focus:outline-none"
            rows={2}
            value={comment}
            onChange={(e) => setComment(e.target.value)}
            placeholder="Reason for rejecting (required)"
          />
          <div className="flex gap-2">
            <button
              type="button"
              disabled={busy || comment.trim() === ""}
              onClick={() => onDecide("reject", comment.trim())}
              className="rounded bg-red-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-red-700 disabled:opacity-50"
            >
              Confirm rejection
            </button>
            <button
              type="button"
              onClick={() => {
                setDeciding(false);
                setComment("");
              }}
              className="rounded px-3 py-1.5 text-xs text-gray-600 hover:bg-gray-100"
            >
              Cancel
            </button>
          </div>
        </div>
      )}

      {/* The requester (while editable) manages the approver. */}
      {canManage && (
        <div className="mt-2 flex gap-3 text-xs">
          {a.status === "rejected" && (
            <button
              type="button"
              disabled={busy}
              onClick={onRequestAgain}
              className="text-indigo-600 hover:underline disabled:opacity-50"
            >
              Request approval again
            </button>
          )}
          <button
            type="button"
            disabled={busy}
            onClick={onRemove}
            className="text-gray-400 hover:text-red-600 disabled:opacity-50"
          >
            Remove approver
          </button>
        </div>
      )}
    </li>
  );
}
