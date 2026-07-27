import type { ReactNode } from "react";
import { Link } from "react-router-dom";
import {
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  CircularProgress,
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
import { Plus } from "@wso2/oxygen-ui-icons-react";
import { StatusBadge } from "./StatusBadge";
import { prReference, type Me, type PurchaseRequest } from "../types/api";

type Props = {
  title: string;
  subtitle: string;
  data: PurchaseRequest[] | undefined;
  isLoading: boolean;
  error: unknown;
  me: Me | undefined;
  // emptyMessage is shown when the list is empty (before the "create one" hint).
  emptyMessage?: string;
  // toolbar renders between the header and the table (e.g. the filter bar).
  toolbar?: ReactNode;
  // filtersActive suppresses the "create your first one" empty card and instead
  // shows a "no matches" hint — the list is empty because of the active filters,
  // not because none exist.
  filtersActive?: boolean;
  // canCreate shows the "New request" affordances (header + empty-state button).
  // New requests are created from "My requests" only, not the procurement queue.
  canCreate?: boolean;
};

// PurchaseRequestsList renders the shared request table used by both the
// "My requests" and "Purchase requests" pages — only the header copy differs.
export function PurchaseRequestsList({
  title,
  subtitle,
  data,
  isLoading,
  error,
  me,
  emptyMessage = "No purchase requests yet",
  toolbar,
  filtersActive = false,
  canCreate = false,
}: Props) {
  return (
    <Box>
      <Box
        sx={{
          mb: 3,
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          gap: 2,
        }}
      >
        <Box>
          <Typography variant="h5" sx={{ fontWeight: 600 }}>
            {title}
          </Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
            {subtitle}
          </Typography>
        </Box>
        {canCreate && (
          <Button
            component={Link}
            to="/requests/new"
            variant="contained"
            startIcon={<Plus size={16} />}
          >
            New request
          </Button>
        )}
      </Box>

      {toolbar}

      {isLoading && (
        <Box sx={{ display: "flex", alignItems: "center", gap: 1, color: "text.secondary" }}>
          <CircularProgress size={18} />
          <Typography variant="body2" color="text.secondary">
            Loading…
          </Typography>
        </Box>
      )}
      {!!error && (
        <Typography variant="body2" color="error.main">
          Failed to load requests.
        </Typography>
      )}

      {data && data.length === 0 && filtersActive && (
        <Card variant="outlined" sx={{ borderStyle: "dashed" }}>
          <CardContent sx={{ py: 6, textAlign: "center" }}>
            <Typography variant="subtitle2">No requests match these filters</Typography>
            <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
              Try clearing or widening the filters above.
            </Typography>
          </CardContent>
        </Card>
      )}

      {data && data.length === 0 && !filtersActive && (
        <Card variant="outlined" sx={{ borderStyle: "dashed" }}>
          <CardContent sx={{ py: 6, textAlign: "center" }}>
            <Typography variant="subtitle2">{emptyMessage}</Typography>
            {canCreate && (
              <>
                <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
                  Create your first one to get started.
                </Typography>
                <Button
                  component={Link}
                  to="/requests/new"
                  variant="contained"
                  startIcon={<Plus size={16} />}
                  sx={{ mt: 2 }}
                >
                  New request
                </Button>
              </>
            )}
          </CardContent>
        </Card>
      )}

      {data && data.length > 0 && (
        <TableContainer component={Paper} variant="outlined">
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell>Ref</TableCell>
                <TableCell>Title</TableCell>
                <TableCell>Status</TableCell>
                <TableCell>Assignee</TableCell>
                <TableCell>Approvals</TableCell>
                <TableCell>Created</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {data.map((pr) => (
                <TableRow key={pr.id} hover>
                  <TableCell>
                    <MuiLink
                      component={Link}
                      to={`/requests/${pr.id}`}
                      sx={{ fontWeight: 600 }}
                    >
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
                  <TableCell>
                    <StatusBadge status={pr.status} />
                  </TableCell>
                  <TableCell>
                    {me && pr.assignee_id === me.id ? (
                      <Chip size="small" variant="outlined" color="primary" label="Assigned to you" />
                    ) : pr.assignee ? (
                      <Typography variant="body2" color="text.secondary">
                        {pr.assignee.name || pr.assignee.email}
                      </Typography>
                    ) : pr.team_lead_status === "approved" ? (
                      <Chip size="small" variant="outlined" color="warning" label="Unassigned" />
                    ) : (
                      <Typography component="span" color="text.disabled">
                        —
                      </Typography>
                    )}
                  </TableCell>
                  <TableCell>
                    {pr.my_approval_status === "pending" ? (
                      <Chip size="small" variant="outlined" color="warning" label="Awaiting you" />
                    ) : pr.approvals_total > 0 ? (
                      <Typography variant="body2" color="text.secondary">
                        {pr.approvals_approved}/{pr.approvals_total} approved
                      </Typography>
                    ) : (
                      <Typography component="span" color="text.disabled">
                        —
                      </Typography>
                    )}
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
