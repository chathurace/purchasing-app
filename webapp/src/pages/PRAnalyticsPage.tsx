import { Link, useParams } from "react-router-dom";
import {
  Alert,
  Box,
  Button,
  Chip,
  CircularProgress,
  Divider,
  Paper,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { ChevronLeft } from "@wso2/oxygen-ui-icons-react";
import { useCanViewAnalytics, usePRFlow } from "../hooks/useAnalytics";
import { AnalyticsAccessDenied } from "../components/AnalyticsAccessDenied";
import { ProcessFlowTimeline } from "../components/ProcessFlowTimeline";
import { StatusBadge } from "../components/StatusBadge";
import { formatDuration } from "../lib/eventLabels";
import { prPriority, prPriorityColor, prReference } from "../types/api";
import type { AnalyticsPR, ProcessEvent, UserSummary } from "../types/api";

// ── Analytics → one purchase request's process flow ──────────────────────────
//
// The request's identity and owners, the three timings derived from its event
// log, then the flow itself (ProcessFlowTimeline). Everything on this page comes
// from one endpoint — it does not go through the gated PR read, so a
// procurement_admin can analyse a request they could not open on the PR page.

// Field is one label/value pair of the header block.
function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <Box sx={{ minWidth: 0 }}>
      <Typography variant="caption" color="text.secondary" sx={{ display: "block" }}>
        {label}
      </Typography>
      <Box sx={{ mt: 0.25 }}>{children}</Box>
    </Box>
  );
}

function person(user: UserSummary | null) {
  if (!user) {
    return (
      <Typography variant="body2" color="text.disabled">
        Unassigned
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

// StatTile — label plus the value, in proportional figures (a standalone number
// looks loose in tabular-nums at this size). No delta and no sparkline: these are
// single facts about one request, not a series.
function StatTile({ label, value, hint }: { label: string; value: string; hint?: string }) {
  return (
    <Paper variant="outlined" sx={{ px: 2, py: 1.5, flex: "1 1 180px", minWidth: 160 }}>
      <Typography variant="caption" color="text.secondary" sx={{ display: "block" }}>
        {label}
      </Typography>
      <Typography variant="h6" sx={{ fontWeight: 600, mt: 0.25 }}>
        {value}
      </Typography>
      {hint && (
        <Typography variant="caption" color="text.secondary">
          {hint}
        </Typography>
      )}
    </Paper>
  );
}

// flowTimings derives the three headline numbers from the event log. All are
// "—" when there are no events; the elapsed span needs at least two.
function flowTimings(events: ProcessEvent[]) {
  if (events.length === 0) return { steps: "0", elapsed: "—", idle: "—", idleHint: undefined };
  const first = new Date(events[0].created_at).getTime();
  const last = new Date(events[events.length - 1].created_at).getTime();
  return {
    steps: String(events.length),
    elapsed: events.length > 1 ? formatDuration(last - first) : "—",
    idle: formatDuration(Date.now() - last),
    idleHint: new Date(last).toLocaleString(),
  };
}

function Header({ pr }: { pr: AnalyticsPR }) {
  return (
    <Paper variant="outlined" sx={{ p: { xs: 2, md: 3 }, mb: 3 }}>
      <Stack
        direction={{ xs: "column", sm: "row" }}
        spacing={2}
        sx={{ alignItems: { sm: "flex-start" }, justifyContent: "space-between" }}
      >
        <Box sx={{ minWidth: 0 }}>
          <Typography variant="h6" sx={{ fontWeight: 600 }}>
            {prReference(pr)}
          </Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mt: 0.25 }}>
            {pr.title || "Untitled request"}
          </Typography>
        </Box>
        <Stack direction="row" spacing={1} sx={{ alignItems: "center", flexShrink: 0 }}>
          <Chip
            size="small"
            variant="outlined"
            color={prPriorityColor(prPriority(pr))}
            label={prPriority(pr)}
          />
          <StatusBadge status={pr.status} />
        </Stack>
      </Stack>

      <Divider sx={{ my: 2 }} />

      <Box
        sx={{
          display: "grid",
          gap: 2,
          gridTemplateColumns: { xs: "1fr 1fr", md: "repeat(3, minmax(0, 1fr))" },
        }}
      >
        <Field label="Created">
          <Typography variant="body2">{new Date(pr.created_at).toLocaleString()}</Typography>
        </Field>
        <Field label="Requester">{person(pr.requester)}</Field>
        <Field label="Assignee">{person(pr.assignee)}</Field>
      </Box>
    </Paper>
  );
}

export function PRAnalyticsPage() {
  const canView = useCanViewAnalytics();
  const { id } = useParams<{ id: string }>();
  const prId = Number(id);
  const { data, isLoading, error } = usePRFlow(prId, canView);

  if (!canView) return <AnalyticsAccessDenied />;

  const timings = flowTimings(data?.events ?? []);

  return (
    <Box sx={{ maxWidth: 1200, mx: "auto", p: { xs: 2, md: 4 } }}>
      <Button
        component={Link}
        to="/analytics/purchase-requests"
        size="small"
        color="inherit"
        startIcon={<ChevronLeft size={16} />}
        sx={{ mb: 2 }}
      >
        Purchase request analytics
      </Button>

      {isLoading && <CircularProgress size={24} />}
      {!!error && <Alert severity="error">Failed to load this request's process flow.</Alert>}

      {data && (
        <>
          <Header pr={data.purchase_request} />

          <Stack direction="row" spacing={2} sx={{ mb: 3, flexWrap: "wrap", rowGap: 2 }}>
            <StatTile label="Steps recorded" value={timings.steps} />
            <StatTile label="First to last step" value={timings.elapsed} />
            <StatTile label="Since last step" value={timings.idle} hint={timings.idleHint} />
          </Stack>

          <Typography variant="subtitle1" sx={{ fontWeight: 600, mb: 0.5 }}>
            Process flow
          </Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 2.5 }}>
            Every recorded step, earliest first. The time on each connector is how long the
            request waited between the two steps it joins.
          </Typography>
          <ProcessFlowTimeline events={data.events} />
        </>
      )}
    </Box>
  );
}
