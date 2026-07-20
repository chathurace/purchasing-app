import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { ApiError } from "../api/client";
import {
  moveTeamLeadToPending,
  recordTeamLeadDecision,
  remindTeamLead,
  updateTeamLeadEmail,
} from "../api/purchaseRequests";
import { APPROVAL_STATUS_LABELS } from "../types/api";
import type { ApprovalStatus, Me, PurchaseRequest } from "../types/api";
import { ConfirmDialog } from "./ConfirmDialog";

const statusCls: Record<ApprovalStatus, string> = {
  pending: "bg-amber-100 text-amber-800",
  approved: "bg-green-100 text-green-800",
  rejected: "bg-red-100 text-red-800",
};

function PencilIcon() {
  return (
    <svg viewBox="0 0 24 24" className="h-4 w-4" fill="none" stroke="currentColor" strokeWidth={1.8}
      strokeLinecap="round" strokeLinejoin="round">
      <path d="M12 20h9" />
      <path d="M16.5 3.5a2.12 2.12 0 0 1 3 3L7 19l-4 1 1-4Z" />
    </svg>
  );
}

// TeamLeadApprovalCard renders the single team-lead sign-off on a PR — the gate
// that lets procurement see and act on it. The team lead (or an admin) records the
// decision with notes; each action is confirmed via a modal. Once decided it
// shows the outcome and timestamp, with an "Edit decision" affordance for the
// actor (which also offers moving the decision back to pending). While the
// approval is still pending, the requester / procurement_admin / admin can change the
// named team lead inline. Everyone else sees a read-only status.
export function TeamLeadApprovalCard({ pr, me }: { pr: PurchaseRequest; me: Me | undefined }) {
  const qc = useQueryClient();
  const decided = pr.team_lead_status !== "pending";
  const actionable = !!pr.my_team_lead_actionable;

  const canEditEmail =
    !!me &&
    (me.id === pr.requester_id ||
      me.roles.includes("procurement_admin") ||
      me.roles.includes("admin"));

  const [editing, setEditing] = useState(false);
  const [notes, setNotes] = useState(pr.team_lead_notes ?? "");
  const [confirm, setConfirm] = useState<"approve" | "reject" | null>(null);
  const [error, setError] = useState<string | null>(null);

  const [editingEmail, setEditingEmail] = useState(false);
  const [emailValue, setEmailValue] = useState(pr.team_lead_email ?? "");
  const [reminded, setReminded] = useState(false);

  // The decision form is shown while pending, or while the actor is editing a
  // prior decision.
  const showForm = actionable && (!decided || editing);

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ["purchase-requests", pr.id] });
    qc.invalidateQueries({ queryKey: ["purchase-requests"] });
  };

  const mutation = useMutation({
    mutationFn: (decision: "approve" | "reject") => recordTeamLeadDecision(pr.id, decision, notes.trim()),
    onSuccess: () => {
      invalidate();
      setEditing(false);
      setConfirm(null);
      setError(null);
    },
    onError: (e: unknown) => {
      setConfirm(null);
      setError(e instanceof ApiError ? e.message : "Failed to record decision");
    },
  });

  const pendingMutation = useMutation({
    mutationFn: () => moveTeamLeadToPending(pr.id),
    onSuccess: () => {
      invalidate();
      setEditing(false);
      setError(null);
    },
    onError: (e: unknown) => setError(e instanceof ApiError ? e.message : "Failed to reopen decision"),
  });

  const emailMutation = useMutation({
    mutationFn: () => updateTeamLeadEmail(pr.id, emailValue.trim()),
    onSuccess: () => {
      invalidate();
      setEditingEmail(false);
      setError(null);
    },
    onError: (e: unknown) => setError(e instanceof ApiError ? e.message : "Failed to update team lead"),
  });

  const reminderMutation = useMutation({
    mutationFn: () => remindTeamLead(pr.id),
    onSuccess: () => {
      setReminded(true);
      setError(null);
    },
    onError: (e: unknown) => setError(e instanceof ApiError ? e.message : "Failed to send reminder"),
  });

  const requestConfirm = (decision: "approve" | "reject") => {
    if (decision === "reject" && !notes.trim()) {
      setError("Notes are required when rejecting.");
      return;
    }
    setError(null);
    setConfirm(decision);
  };

  return (
    <div className="mt-6 rounded border bg-white p-6">
      <div className="mb-3 flex items-center justify-between">
        <h2 className="font-medium text-gray-900">Team lead approval</h2>
        <span className={`rounded px-2 py-0.5 text-xs font-medium ${statusCls[pr.team_lead_status]}`}>
          {APPROVAL_STATUS_LABELS[pr.team_lead_status]}
        </span>
      </div>

      {editingEmail ? (
        <div className="flex flex-wrap items-center gap-2">
          <label className="text-sm text-gray-600">Team lead:</label>
          <input
            type="email"
            className="min-w-[16rem] flex-1 rounded border px-2 py-1 text-sm focus:border-indigo-500 focus:outline-none"
            value={emailValue}
            onChange={(e) => setEmailValue(e.target.value)}
            placeholder="team.lead@example.com"
            autoFocus
          />
          <button
            type="button"
            className="rounded bg-indigo-600 px-3 py-1 text-sm font-medium text-white hover:bg-indigo-700 disabled:opacity-50"
            onClick={() => emailMutation.mutate()}
            disabled={emailMutation.isPending || !emailValue.trim()}
          >
            {emailMutation.isPending ? "Saving…" : "Save"}
          </button>
          <button
            type="button"
            className="rounded border px-3 py-1 text-sm text-gray-700 hover:bg-gray-50"
            onClick={() => {
              setEditingEmail(false);
              setEmailValue(pr.team_lead_email ?? "");
              setError(null);
            }}
            disabled={emailMutation.isPending}
          >
            Cancel
          </button>
        </div>
      ) : (
        <p className="flex items-center gap-2 text-sm text-gray-600">
          Team lead: <span className="font-medium text-gray-900">{pr.team_lead_email || "—"}</span>
          {canEditEmail && !decided && (
            <button
              type="button"
              className="text-gray-400 hover:text-indigo-600"
              title="Change team lead"
              onClick={() => {
                setEmailValue(pr.team_lead_email ?? "");
                setEditingEmail(true);
              }}
            >
              <PencilIcon />
            </button>
          )}
        </p>
      )}

      {canEditEmail && !decided && !editingEmail && pr.team_lead_email && (
        <div className="mt-2">
          <button
            type="button"
            disabled={reminderMutation.isPending}
            onClick={() => {
              setReminded(false);
              reminderMutation.mutate();
            }}
            className="rounded border px-3 py-1 text-xs font-medium text-indigo-600 hover:bg-indigo-50 disabled:opacity-50"
          >
            {reminderMutation.isPending ? "Sending…" : reminded ? "Reminder sent ✓" : "Send reminder"}
          </button>
        </div>
      )}

      {decided && pr.team_lead_decided_at && (
        <p className="mt-1 text-sm text-gray-500">
          {pr.team_lead_status === "approved" ? "Approved" : "Rejected"} on{" "}
          {new Date(pr.team_lead_decided_at).toLocaleString()}
        </p>
      )}
      {decided && pr.team_lead_notes && !showForm && (
        <p className="mt-2 whitespace-pre-wrap rounded bg-gray-50 p-3 text-sm text-gray-700">
          {pr.team_lead_notes}
        </p>
      )}

      {!actionable && !decided && (
        <p className="mt-2 text-sm text-gray-400">Awaiting the team lead's decision.</p>
      )}

      {showForm && (
        <div className="mt-4 space-y-3">
          <textarea
            className="min-h-[80px] w-full rounded border px-3 py-2 text-sm focus:border-indigo-500 focus:outline-none"
            value={notes}
            onChange={(e) => setNotes(e.target.value)}
            placeholder="Notes (required when rejecting)"
          />
          <div className="flex flex-wrap gap-2">
            <button
              type="button"
              className="rounded bg-green-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-green-700 disabled:opacity-50"
              onClick={() => requestConfirm("approve")}
              disabled={mutation.isPending || pendingMutation.isPending}
            >
              Approve
            </button>
            <button
              type="button"
              className="rounded bg-red-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-red-700 disabled:opacity-50"
              onClick={() => requestConfirm("reject")}
              disabled={mutation.isPending || pendingMutation.isPending}
            >
              Reject
            </button>
            {decided && editing && (
              <button
                type="button"
                className="rounded border border-amber-300 px-3 py-1.5 text-sm font-medium text-amber-700 hover:bg-amber-50 disabled:opacity-50"
                onClick={() => pendingMutation.mutate()}
                disabled={mutation.isPending || pendingMutation.isPending}
              >
                {pendingMutation.isPending ? "Working…" : "Move to pending"}
              </button>
            )}
            {editing && (
              <button
                type="button"
                className="rounded border px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-50"
                onClick={() => {
                  setEditing(false);
                  setNotes(pr.team_lead_notes ?? "");
                  setError(null);
                }}
                disabled={mutation.isPending || pendingMutation.isPending}
              >
                Cancel
              </button>
            )}
          </div>
        </div>
      )}

      {actionable && decided && !editing && (
        <button
          type="button"
          className="mt-4 text-sm font-medium text-indigo-600 hover:text-indigo-700"
          onClick={() => {
            setNotes(pr.team_lead_notes ?? "");
            setEditing(true);
          }}
        >
          Edit decision
        </button>
      )}

      {error && <p className="mt-2 text-sm text-red-600">{error}</p>}

      {confirm && (
        <ConfirmDialog
          title={confirm === "approve" ? "Approve this request?" : "Reject this request?"}
          message={
            confirm === "approve"
              ? "Approving lets procurement see and start working on this purchase request."
              : "Rejecting keeps this request hidden from procurement. The requester can edit and resubmit it."
          }
          confirmLabel={confirm === "approve" ? "Approve" : "Reject"}
          danger={confirm === "reject"}
          busy={mutation.isPending}
          onConfirm={() => mutation.mutate(confirm)}
          onCancel={() => setConfirm(null)}
        />
      )}
    </div>
  );
}
