import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import {
  Alert,
  Box,
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
  TableSortLabel,
  Typography,
} from "@wso2/oxygen-ui";
import { useAnalyticsPRs, useCanViewAnalytics } from "../hooks/useAnalytics";
import { AnalyticsAccessDenied } from "../components/AnalyticsAccessDenied";
import { prPriority, prPriorityColor, prReference } from "../types/api";
import type { AnalyticsPR, AnalyticsPRSort } from "../types/api";

// ── Analytics → Purchase requests (docs/bpm-analytics.md) ────────────────────
//
// Every purchase request in the organisation, one row each, as the entry point to
// its process flow. Sorting is server-side (the list is capped at a page, so
// sorting locally would only reorder the page rather than the data) — a header
// click changes the query, not the rendered array.

type SortState = { sort: AnalyticsPRSort; dir: "asc" | "desc" };

// naturalDir is the direction a column reads as on its first click: newest-first
// for a timestamp, A→Z for a person. Mirrors the server's own default.
function naturalDir(sort: AnalyticsPRSort): "asc" | "desc" {
  return sort === "created_at" ? "desc" : "asc";
}

const COLUMNS: { sort: AnalyticsPRSort; label: string }[] = [
  { sort: "created_at", label: "Created" },
  { sort: "requester", label: "Requester" },
  { sort: "assignee", label: "Assignee" },
];

function personCell(user: AnalyticsPR["requester"]) {
  if (!user) {
    return (
      <Typography component="span" color="text.disabled">
        —
      </Typography>
    );
  }
  return (
    <>
      <Typography variant="body2">{user.name || user.email}</Typography>
      {user.name && user.email && (
        <Typography variant="caption" color="text.secondary">
          {user.email}
        </Typography>
      )}
    </>
  );
}

export function AnalyticsPRListPage() {
  const canView = useCanViewAnalytics();
  const navigate = useNavigate();
  const [{ sort, dir }, setSort] = useState<SortState>({ sort: "created_at", dir: "desc" });
  const { data, isLoading, error, isFetching } = useAnalyticsPRs(sort, dir, canView);

  // Clicking the active column flips it; a new column starts at its natural
  // direction rather than inheriting the previous column's.
  const onSort = (next: AnalyticsPRSort) =>
    setSort((prev) =>
      prev.sort === next
        ? { sort: next, dir: prev.dir === "asc" ? "desc" : "asc" }
        : { sort: next, dir: naturalDir(next) },
    );

  if (!canView) return <AnalyticsAccessDenied />;

  return (
    <Box sx={{ maxWidth: 1200, mx: "auto", p: { xs: 2, md: 4 } }}>
      <Typography variant="h5" sx={{ fontWeight: 600 }}>
        Purchase request analytics
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5, mb: 3 }}>
        Every purchase request and who owns it. Open one to see its process flow — every
        recorded step, in order, with the time spent between them.
      </Typography>

      {isLoading && <CircularProgress size={24} />}
      {!!error && <Alert severity="error">Failed to load purchase requests.</Alert>}

      {data && (
        <>
          <Typography variant="caption" color="text.secondary" sx={{ display: "block", mb: 1 }}>
            {data.length === 2000
              ? "Showing the latest 2000 requests."
              : `${data.length} request${data.length === 1 ? "" : "s"}`}
            {isFetching ? " · updating…" : ""}
          </Typography>

          {data.length === 0 ? (
            <Card variant="outlined" sx={{ borderStyle: "dashed" }}>
              <CardContent sx={{ py: 6, textAlign: "center" }}>
                <Typography variant="subtitle2">No purchase requests yet</Typography>
              </CardContent>
            </Card>
          ) : (
            <TableContainer component={Paper} variant="outlined">
              <Table size="small">
                <TableHead>
                  <TableRow>
                    <TableCell>Request</TableCell>
                    <TableCell>Title</TableCell>
                    <TableCell>Priority</TableCell>
                    {COLUMNS.map((c) => (
                      <TableCell
                        key={c.sort}
                        sortDirection={sort === c.sort ? dir : false}
                        sx={{ whiteSpace: "nowrap" }}
                      >
                        <TableSortLabel
                          active={sort === c.sort}
                          direction={sort === c.sort ? dir : naturalDir(c.sort)}
                          onClick={() => onSort(c.sort)}
                        >
                          {c.label}
                        </TableSortLabel>
                      </TableCell>
                    ))}
                  </TableRow>
                </TableHead>
                <TableBody>
                  {data.map((pr) => (
                    <TableRow
                      key={pr.id}
                      hover
                      onClick={() => navigate(`/analytics/purchase-requests/${pr.id}`)}
                      sx={{ cursor: "pointer" }}
                    >
                      <TableCell>
                        {/* A real link as well as the row click, so the reference
                            stays keyboard-reachable and middle-clickable. */}
                        <MuiLink
                          component={Link}
                          to={`/analytics/purchase-requests/${pr.id}`}
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
                        <Chip
                          size="small"
                          variant="outlined"
                          color={prPriorityColor(prPriority(pr))}
                          label={prPriority(pr)}
                        />
                      </TableCell>
                      <TableCell sx={{ whiteSpace: "nowrap", color: "text.secondary" }}>
                        {new Date(pr.created_at).toLocaleString()}
                      </TableCell>
                      <TableCell>{personCell(pr.requester)}</TableCell>
                      <TableCell>{personCell(pr.assignee)}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableContainer>
          )}
        </>
      )}
    </Box>
  );
}
