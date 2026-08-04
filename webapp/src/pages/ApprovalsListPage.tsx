import { useState } from "react";
import { Link } from "react-router-dom";
import {
  Alert,
  Box,
  Checkbox,
  Chip,
  CircularProgress,
  FormControlLabel,
  Link as MuiLink,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
} from "@wso2/oxygen-ui";
import { useApprovalRequests } from "../hooks/usePurchaseRequests";
import { StatusBadge } from "../components/StatusBadge";
import { prReference, type ApprovalStatus } from "../types/api";

// MyApprovalBadge renders the caller's own state on a PR awaiting them.
function MyApprovalBadge({ state }: { state?: ApprovalStatus | null }) {
  if (state === "pending") {
    return <Chip size="small" variant="outlined" color="warning" label="Awaiting you" />;
  }
  if (state === "approved") {
    return <Chip size="small" variant="outlined" color="success" label="Approved" />;
  }
  if (state === "rejected") {
    return <Chip size="small" variant="outlined" color="error" label="Rejected" />;
  }
  return (
    <Typography component="span" color="text.disabled">
      —
    </Typography>
  );
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
    <Box sx={{ maxWidth: 1200, mx: "auto", p: { xs: 2, md: 4 } }}>
      <Box sx={{ mb: 3 }}>
        <Typography variant="h5" sx={{ fontWeight: 600 }}>
          Approvals
        </Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
          Purchase requests awaiting your decision as a team lead, or a budget, legal, security, or
          compliance approver.
        </Typography>
      </Box>

      <Box sx={{ mb: 2, display: "flex", alignItems: "center", gap: 3 }}>
        <FormControlLabel
          control={
            <Checkbox
              size="small"
              checked={showPending}
              onChange={(e) => setShowPending(e.target.checked)}
            />
          }
          label="Pending"
        />
        <FormControlLabel
          control={
            <Checkbox
              size="small"
              checked={showReviewed}
              onChange={(e) => setShowReviewed(e.target.checked)}
            />
          }
          label="Approved"
        />
      </Box>

      {isLoading && <CircularProgress size={24} />}
      {error && <Alert severity="error">Failed to load approvals.</Alert>}

      {data && rows.length === 0 && (
        <Paper variant="outlined" sx={{ p: 6, textAlign: "center", borderStyle: "dashed" }}>
          <Typography variant="subtitle2">Nothing to show</Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
            {data.length === 0
              ? "No purchase requests are awaiting your approval."
              : "No requests match the selected filters."}
          </Typography>
        </Paper>
      )}

      {rows.length > 0 && (
        <TableContainer component={Paper} variant="outlined">
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell>Ref</TableCell>
                <TableCell>Title</TableCell>
                <TableCell>Requester</TableCell>
                <TableCell>Status</TableCell>
                <TableCell>Your decision</TableCell>
                <TableCell>Created</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {rows.map((pr) => (
                <TableRow key={pr.id} hover>
                  <TableCell>
                    <MuiLink component={Link} to={`/requests/${pr.id}`} sx={{ fontWeight: 500 }}>
                      {prReference(pr)}
                    </MuiLink>
                  </TableCell>
                  <TableCell>
                    {pr.title || (
                      <Typography component="span" color="text.disabled">
                        —
                      </Typography>
                    )}
                  </TableCell>
                  <TableCell sx={{ color: "text.secondary" }}>
                    {pr.requester?.name || pr.requester?.email || "—"}
                  </TableCell>
                  <TableCell>
                    <StatusBadge status={pr.status} />
                  </TableCell>
                  <TableCell>
                    <MyApprovalBadge state={pr.my_approval_state} />
                  </TableCell>
                  <TableCell sx={{ color: "text.secondary" }}>
                    {new Date(pr.created_at).toLocaleDateString()}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}
    </Box>
  );
}
