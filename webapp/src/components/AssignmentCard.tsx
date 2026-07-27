import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  MenuItem,
  Stack,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { ApiError } from "../api/client";
import {
  addPRCollaborator,
  removePRCollaborator,
  setPRAssignee,
} from "../api/purchaseRequests";
import { listTeams } from "../api/teams";
import type { Me, PurchaseRequest, UserSummary } from "../types/api";
import { ApproverPicker } from "./ApproverPicker";

function userLabel(u: UserSummary): string {
  return u.name ? `${u.name} (${u.email})` : u.email || `#${u.id}`;
}

// AssignmentCard renders the PR-assignment gate that sits between team-lead approval
// and procurement work. A procurement user is the PR's assignee; zero or more
// collaborators share full work access. Any procurement user can claim an unassigned
// PR (or hand back their own); a procurement_admin/admin can assign/reassign anyone
// via the dropdown and manage collaborators. Only shown once the team lead has
// approved (before then procurement can't see the PR at all).
export function AssignmentCard({ pr, me }: { pr: PurchaseRequest; me: Me | undefined }) {
  const qc = useQueryClient();
  const [error, setError] = useState<string | null>(null);

  const isProcurementAdmin =
    !!me && (me.roles.includes("procurement_admin") || me.roles.includes("admin"));
  const canAssign = !!pr.my_can_assign;
  const canManage = !!pr.my_can_manage_collaborators;

  // Only relevant once the team lead has approved; hidden otherwise. Requesters and
  // other viewers with no controls still see who (if anyone) is handling the PR.
  const visible = pr.team_lead_status === "approved" && (canAssign || canManage || !!pr.assignee || !!me);

  const { data: teams } = useQuery({
    queryKey: ["teams"],
    queryFn: listTeams,
    enabled: visible && (canAssign || canManage),
  });
  // Assignable pool = the Procurement team (plain `procurement` role holders) plus
  // the current user when they're a procurement_admin/admin but not already a team
  // member — so a procurement_admin can pick (assign to) themselves.
  const members: UserSummary[] = useMemo(() => {
    const teamMembers = teams?.find((t) => t.key === "procurement")?.members ?? [];
    if (me && isProcurementAdmin && !teamMembers.some((m) => m.id === me.id)) {
      return [{ id: me.id, email: me.email, name: me.name }, ...teamMembers];
    }
    return teamMembers;
  }, [teams, me, isProcurementAdmin]);

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ["purchase-requests", pr.id] });
    qc.invalidateQueries({ queryKey: ["purchase-requests"] });
  };

  const assignMutation = useMutation({
    mutationFn: (assigneeId: number | null) => setPRAssignee(pr.id, assigneeId),
    onSuccess: () => {
      invalidate();
      setError(null);
    },
    onError: (e: unknown) => setError(e instanceof ApiError ? e.message : "Failed to update assignee"),
  });

  const addCollab = useMutation({
    mutationFn: (userId: number) => addPRCollaborator(pr.id, userId),
    onSuccess: () => {
      invalidate();
      setError(null);
    },
    onError: (e: unknown) => setError(e instanceof ApiError ? e.message : "Failed to add collaborator"),
  });

  const removeCollab = useMutation({
    mutationFn: (userId: number) => removePRCollaborator(pr.id, userId),
    onSuccess: () => {
      invalidate();
      setError(null);
    },
    onError: (e: unknown) => setError(e instanceof ApiError ? e.message : "Failed to remove collaborator"),
  });

  if (!visible) return null;

  const busy = assignMutation.isPending;
  const assigned = !!pr.assignee_id;
  const collaborators = pr.collaborators ?? [];
  // Assignee + existing collaborators are hidden from the collaborator add-list.
  const collabSelectedIds = [
    ...(pr.assignee_id ? [pr.assignee_id] : []),
    ...collaborators.map((c) => c.id),
  ];

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
          <Typography variant="h6">Assignment</Typography>
          <Chip
            size="small"
            variant="outlined"
            color={assigned ? "success" : "warning"}
            label={assigned ? "Assigned" : "Unassigned"}
          />
        </Box>

        {/* Assignee */}
        <Box sx={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 1 }}>
          <Typography variant="body2" color="text.secondary">
            Assignee:
          </Typography>
          <Typography variant="body2" sx={{ fontWeight: 600 }}>
            {pr.assignee ? userLabel(pr.assignee) : "—"}
          </Typography>
        </Box>

        {canAssign && (
          <Box sx={{ mt: 1, display: "flex", flexWrap: "wrap", alignItems: "center", gap: 1 }}>
            {isProcurementAdmin ? (
              <TextField
                select
                size="small"
                value={pr.assignee_id ?? ""}
                disabled={busy}
                onChange={(e) =>
                  assignMutation.mutate(e.target.value === "" ? null : Number(e.target.value))
                }
                sx={{ minWidth: "16rem" }}
              >
                <MenuItem value="">Unassigned</MenuItem>
                {/* Keep a stale assignee visible even if they're not in the member list. */}
                {pr.assignee && !members.some((m) => m.id === pr.assignee_id) && (
                  <MenuItem value={pr.assignee_id ?? ""}>{userLabel(pr.assignee)}</MenuItem>
                )}
                {members.map((m) => (
                  <MenuItem key={m.id} value={m.id}>
                    {userLabel(m)}
                  </MenuItem>
                ))}
              </TextField>
            ) : !assigned ? (
              <Button
                variant="contained"
                onClick={() => me && assignMutation.mutate(me.id)}
                disabled={busy}
              >
                {busy ? "Assigning…" : "Assign to me"}
              </Button>
            ) : (
              me &&
              pr.assignee_id === me.id && (
                <Button
                  variant="outlined"
                  color="inherit"
                  onClick={() => assignMutation.mutate(null)}
                  disabled={busy}
                >
                  {busy ? "Working…" : "Unassign me"}
                </Button>
              )
            )}
          </Box>
        )}

        {!assigned && canAssign && (
          <Typography variant="body2" color="text.disabled" sx={{ mt: 1 }}>
            This request must be assigned to a procurement user before quotations or a
            recommendation can be added.
          </Typography>
        )}

        {/* Collaborators */}
        <Box sx={{ mt: 2 }}>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 0.5 }}>
            Collaborators
          </Typography>
          {canManage ? (
            <ApproverPicker
              candidates={members}
              selectedIds={collabSelectedIds}
              selectedUsers={collaborators}
              onAdd={(u) => addCollab.mutate(u.id)}
              onRemove={(id) => removeCollab.mutate(id)}
              disabled={addCollab.isPending || removeCollab.isPending}
            />
          ) : collaborators.length > 0 ? (
            <Stack direction="row" spacing={1} sx={{ flexWrap: "wrap", gap: 1 }}>
              {collaborators.map((c) => (
                <Chip key={c.id} size="small" variant="outlined" label={userLabel(c)} />
              ))}
            </Stack>
          ) : (
            <Typography variant="body2" color="text.disabled">
              No collaborators.
            </Typography>
          )}
        </Box>

        {error && (
          <Alert severity="error" sx={{ mt: 2 }}>
            {error}
          </Alert>
        )}
      </CardContent>
    </Card>
  );
}
