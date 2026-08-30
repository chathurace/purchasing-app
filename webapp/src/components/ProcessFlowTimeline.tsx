import { Fragment } from "react";
import { Box, Chip, Paper, Stack, Typography } from "@wso2/oxygen-ui";
import { Check, Clock, Dot, RotateCcw, X } from "@wso2/oxygen-ui-icons-react";
import { eventOutcome, formatDuration, humanizeEventToken } from "../lib/eventLabels";
import type { EventOutcome } from "../lib/eventLabels";
import type { ProcessEvent } from "../types/api";

// ── Process-flow timeline (docs/bpm-analytics.md) ────────────────────────────
//
// Draws a PR's process events as a single top-to-bottom flow: one node per event
// on a continuous rail, oldest at the top, with the waiting time between
// consecutive steps rendered on the rail segment that separates them. The gaps
// are the point — a BPM view is read to find where a request sat.
//
// Colour carries *outcome* only, and only from the reserved status roles
// (success / error / warning); every other step is neutral. A status colour never
// carries meaning alone here: each node pairs it with an icon and, where the
// event has one, a visible qualifier chip. Everything textual — action, actor,
// timestamp — wears text tokens, so the theme handles light and dark and the
// colour left on screen is only the decisions.

const RAIL_WIDTH = 32; // gutter the rail + node tiles live in
const TILE_SIZE = 26;

// outcomeStyle maps an outcome to its theme palette role. Tiles are outlined
// rather than filled so the glyph keeps its contrast in both themes without
// hand-picking a contrast text colour per palette step.
function outcomeStyle(outcome: EventOutcome): {
  color: string;
  chip: "success" | "error" | "warning" | "default";
  Icon: typeof Check;
} {
  switch (outcome) {
    case "approved":
      return { color: "success.main", chip: "success", Icon: Check };
    case "rejected":
      return { color: "error.main", chip: "error", Icon: X };
    case "reverted":
      return { color: "warning.main", chip: "warning", Icon: RotateCcw };
    default:
      return { color: "text.secondary", chip: "default", Icon: Dot };
  }
}

function formatWhen(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString();
}

// RailSegment is the connector between two nodes. It also hosts the elapsed-time
// label, so the waiting time reads as a property of the gap rather than of either
// step it sits between.
function RailSegment({ ms }: { ms: number }) {
  const label = formatDuration(ms);
  return (
    <Box sx={{ display: "flex", alignItems: "stretch", minHeight: 34 }}>
      <Box sx={{ width: RAIL_WIDTH, display: "flex", justifyContent: "center", flexShrink: 0 }}>
        <Box sx={{ width: 2, bgcolor: "divider" }} />
      </Box>
      {label && (
        <Stack
          direction="row"
          spacing={0.5}
          sx={{ alignItems: "center", alignSelf: "center", pl: 2, color: "text.disabled" }}
        >
          <Clock size={12} />
          <Typography variant="caption" color="text.secondary">
            {label}
          </Typography>
        </Stack>
      )}
    </Box>
  );
}

function EventNode({ event }: { event: ProcessEvent }) {
  const outcome = eventOutcome(event.qualifier);
  const { color, chip, Icon } = outcomeStyle(outcome);
  const actor = event.actor_name || event.actor_email;

  return (
    <Box sx={{ display: "flex", alignItems: "flex-start" }}>
      <Box sx={{ width: RAIL_WIDTH, display: "flex", justifyContent: "center", flexShrink: 0 }}>
        <Box
          sx={{
            width: TILE_SIZE,
            height: TILE_SIZE,
            borderRadius: "50%",
            border: 2,
            borderColor: outcome === "neutral" ? "divider" : color,
            bgcolor: "background.paper",
            color,
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            flexShrink: 0,
          }}
        >
          <Icon size={outcome === "neutral" ? 18 : 14} />
        </Box>
      </Box>

      <Paper variant="outlined" sx={{ flex: 1, minWidth: 0, ml: 2, px: 2, py: 1.25 }}>
        <Stack
          direction={{ xs: "column", sm: "row" }}
          spacing={{ xs: 0.5, sm: 2 }}
          sx={{ alignItems: { sm: "baseline" }, justifyContent: "space-between" }}
        >
          <Stack direction="row" spacing={0.75} sx={{ alignItems: "center", flexWrap: "wrap" }}>
            <Typography variant="body2" sx={{ fontWeight: 600 }}>
              {humanizeEventToken(event.action)}
            </Typography>
            {event.qualifier && (
              <Chip size="small" variant="outlined" color={chip} label={event.qualifier} />
            )}
          </Stack>
          <Typography variant="caption" color="text.secondary" sx={{ whiteSpace: "nowrap" }}>
            {formatWhen(event.created_at)}
          </Typography>
        </Stack>
        <Typography variant="caption" color="text.secondary" sx={{ display: "block", mt: 0.25 }}>
          {actor || "—"}
        </Typography>
      </Paper>
    </Box>
  );
}

// ProcessFlowTimeline renders `events` in the order given — the server returns a
// PR's process events oldest-first, which is the flow direction.
export function ProcessFlowTimeline({ events }: { events: ProcessEvent[] }) {
  if (events.length === 0) {
    return (
      <Paper variant="outlined" sx={{ borderStyle: "dashed", py: 6, textAlign: "center" }}>
        <Typography variant="subtitle2">No process events recorded</Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
          Nothing has happened on this request yet, or it predates the event log.
        </Typography>
      </Paper>
    );
  }

  return (
    <Box sx={{ display: "flex", flexDirection: "column" }}>
      {events.map((e, i) => (
        <Fragment key={e.id}>
          {i > 0 && (
            <RailSegment
              ms={
                new Date(e.created_at).getTime() - new Date(events[i - 1].created_at).getTime()
              }
            />
          )}
          <EventNode event={e} />
        </Fragment>
      ))}
    </Box>
  );
}
