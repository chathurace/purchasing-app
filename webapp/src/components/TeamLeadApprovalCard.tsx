import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  IconButton,
  Stack,
  TextField,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { Pencil } from "@wso2/oxygen-ui-icons-react";
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
import { EmailAutocomplete } from "./EmailAutocomplete";
import { useDirectory } from "../hooks/useDirectory";

type ChipColor = "default" | "success" | "error" | "warning";

const statusColor: Record<ApprovalStatus, ChipColor> = {
  pending: "warning",
  approved: "success",
  rejected: "error",
};

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
  const { data: directory, isLoading: dirLoading, refresh: refreshDir } = useDirectory(editingEmail);
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
    <Card variant="outlined" sx={{ mt: 3 }}>
      <CardContent>
        <Box
          sx={{
            mb: 1.5,
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            gap: 1,
          }}
        >
          <Typography variant="h6">Team lead approval</Typography>
          <Chip
            size="small"
            variant="outlined"
            color={statusColor[pr.team_lead_status] ?? "default"}
            label={APPROVAL_STATUS_LABELS[pr.team_lead_status]}
          />
        </Box>

        {editingEmail ? (
          <Box sx={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 1 }}>
            <Typography variant="body2" color="text.secondary">
              Team lead:
            </Typography>
            <Box sx={{ minWidth: "16rem", flex: 1 }}>
              <EmailAutocomplete
                value={emailValue}
                onChange={(email) => setEmailValue(email)}
                directory={directory ?? []}
                refresh={refreshDir}
                loading={dirLoading}
                placeholder="team.lead@example.com"
                ariaLabel="Team lead email"
              />
            </Box>
            <Button
              variant="contained"
              onClick={() => emailMutation.mutate()}
              disabled={emailMutation.isPending || !emailValue.trim()}
            >
              {emailMutation.isPending ? "Saving…" : "Save"}
            </Button>
            <Button
              variant="outlined"
              color="inherit"
              onClick={() => {
                setEditingEmail(false);
                setEmailValue(pr.team_lead_email ?? "");
                setError(null);
              }}
              disabled={emailMutation.isPending}
            >
              Cancel
            </Button>
          </Box>
        ) : (
          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            <Typography variant="body2" color="text.secondary">
              Team lead:{" "}
              <Typography component="span" variant="body2" sx={{ fontWeight: 600 }} color="text.primary">
                {pr.team_lead_email || "—"}
              </Typography>
            </Typography>
            {canEditEmail && !decided && (
              <Tooltip title="Change team lead">
                <IconButton
                  size="small"
                  onClick={() => {
                    setEmailValue(pr.team_lead_email ?? "");
                    setEditingEmail(true);
                  }}
                >
                  <Pencil size={16} />
                </IconButton>
              </Tooltip>
            )}
          </Box>
        )}

        {canEditEmail && !decided && !editingEmail && pr.team_lead_email && (
          <Box sx={{ mt: 1 }}>
            <Button
              size="small"
              variant="outlined"
              disabled={reminderMutation.isPending}
              onClick={() => {
                setReminded(false);
                reminderMutation.mutate();
              }}
            >
              {reminderMutation.isPending ? "Sending…" : reminded ? "Reminder sent ✓" : "Send reminder"}
            </Button>
          </Box>
        )}

        {decided && pr.team_lead_decided_at && (
          <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
            {pr.team_lead_status === "approved" ? "Approved" : "Rejected"} on{" "}
            {new Date(pr.team_lead_decided_at).toLocaleString()}
          </Typography>
        )}
        {decided && pr.team_lead_notes && !showForm && (
          <Typography
            variant="body2"
            color="text.secondary"
            sx={{
              mt: 1,
              whiteSpace: "pre-wrap",
              bgcolor: "background.default",
              borderRadius: 1,
              p: 1.5,
            }}
          >
            {pr.team_lead_notes}
          </Typography>
        )}

        {!actionable && !decided && (
          <Typography variant="body2" color="text.disabled" sx={{ mt: 1 }}>
            Awaiting the team lead's decision.
          </Typography>
        )}

        {showForm && (
          <Stack spacing={1.5} sx={{ mt: 2 }}>
            <TextField
              fullWidth
              multiline
              minRows={3}
              size="small"
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
              placeholder="Notes (required when rejecting)"
            />
            <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1 }}>
              <Button
                variant="contained"
                color="success"
                onClick={() => requestConfirm("approve")}
                disabled={mutation.isPending || pendingMutation.isPending}
              >
                Approve
              </Button>
              <Button
                variant="contained"
                color="error"
                onClick={() => requestConfirm("reject")}
                disabled={mutation.isPending || pendingMutation.isPending}
              >
                Reject
              </Button>
              {decided && editing && (
                <Button
                  variant="outlined"
                  color="warning"
                  onClick={() => pendingMutation.mutate()}
                  disabled={mutation.isPending || pendingMutation.isPending}
                >
                  {pendingMutation.isPending ? "Working…" : "Move to pending"}
                </Button>
              )}
              {editing && (
                <Button
                  variant="outlined"
                  color="inherit"
                  onClick={() => {
                    setEditing(false);
                    setNotes(pr.team_lead_notes ?? "");
                    setError(null);
                  }}
                  disabled={mutation.isPending || pendingMutation.isPending}
                >
                  Cancel
                </Button>
              )}
            </Box>
          </Stack>
        )}

        {actionable && decided && !editing && (
          <Button
            variant="text"
            sx={{ mt: 2 }}
            onClick={() => {
              setNotes(pr.team_lead_notes ?? "");
              setEditing(true);
            }}
          >
            Edit decision
          </Button>
        )}

        {error && (
          <Alert severity="error" sx={{ mt: 2 }}>
            {error}
          </Alert>
        )}

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
      </CardContent>
    </Card>
  );
}
