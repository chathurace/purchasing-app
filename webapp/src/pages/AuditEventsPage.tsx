import { useEffect, useState } from "react";
import { Link as RouterLink } from "react-router-dom";
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  CircularProgress,
  MenuItem,
  Paper,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { useCanViewAuditLog } from "../hooks/useCanViewAuditLog";
import { useAuditEvents, useEventActions, useProcessEvents } from "../hooks/useEvents";
import type { AuditEvent, EventFilters, ProcessEvent } from "../types/api";

// ── Audit events view (issue #2502) ─────────────────────────────────────────
//
// A read-only admin window onto the two append-only logs: "Process events"
// (per-PR business-process tasks) and "System events" (master-data/admin
// mutations). One section is shown at a time via a segmented control (the UI kit
// has no Tabs). Each section carries its own filters — actor, action, date range,
// and (process only) PR id — applied server-side and debounced.

type Section = "process" | "system";

// humanize turns a snake_case action/entity token into a readable label. Keeping
// this derived (not a hand-maintained map) means the dropdowns stay in sync with
// the Go action catalog automatically.
function humanize(s: string): string {
  if (!s) return "";
  return s.replace(/_/g, " ").replace(/^\w/, (c) => c.toUpperCase());
}

function formatWhen(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString();
}

// useDebounced returns value after it has been stable for `delay` ms — so typing
// in the actor / PR-id fields doesn't fire a request per keystroke.
function useDebounced<T>(value: T, delay = 300): T {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const t = setTimeout(() => setDebounced(value), delay);
    return () => clearTimeout(t);
  }, [value, delay]);
  return debounced;
}

// ActionChip renders the qualifier (approve/reject/sign/…) next to an action when
// present — the decision direction of the task.
function ActionCell({ action, qualifier }: { action: string; qualifier: string }) {
  return (
    <Stack direction="row" spacing={0.75} sx={{ alignItems: "center", flexWrap: "wrap" }}>
      <Typography variant="body2" sx={{ fontWeight: 500 }}>
        {humanize(action)}
      </Typography>
      {qualifier && <Chip size="small" variant="outlined" label={qualifier} />}
    </Stack>
  );
}

function ActorCell({ email, name }: { email: string; name: string }) {
  return (
    <>
      <Typography variant="body2">{name || email || "—"}</Typography>
      {name && email && (
        <Typography variant="caption" color="text.secondary">
          {email}
        </Typography>
      )}
    </>
  );
}

// FilterBar — the shared filter controls. PR id is rendered only for the process
// section. `actions` is the section's action catalog.
function FilterBar({
  filters,
  onChange,
  actions,
  showPRID,
}: {
  filters: EventFilters;
  onChange: (next: EventFilters) => void;
  actions: string[];
  showPRID: boolean;
}) {
  const set = (patch: Partial<EventFilters>) => onChange({ ...filters, ...patch });
  const isEmpty = !Object.values(filters).some((v) => v && String(v).trim() !== "");

  return (
    <Stack direction="row" spacing={1.5} sx={{ mb: 2, flexWrap: "wrap", alignItems: "center" }}>
      <TextField
        size="small"
        label="Actor"
        placeholder="name or email"
        value={filters.actor ?? ""}
        onChange={(e) => set({ actor: e.target.value })}
        sx={{ width: 220 }}
      />
      <TextField
        select
        size="small"
        label="Action"
        value={filters.action ?? ""}
        onChange={(e) => set({ action: e.target.value })}
        SelectProps={{ displayEmpty: true }}
        InputLabelProps={{ shrink: true }}
        sx={{ minWidth: 200 }}
      >
        <MenuItem value="">All actions</MenuItem>
        {actions.map((a) => (
          <MenuItem key={a} value={a}>
            {humanize(a)}
          </MenuItem>
        ))}
      </TextField>
      <TextField
        size="small"
        label="From"
        type="date"
        value={filters.from ?? ""}
        onChange={(e) => set({ from: e.target.value })}
        InputLabelProps={{ shrink: true }}
        sx={{ width: 170 }}
      />
      <TextField
        size="small"
        label="To"
        type="date"
        value={filters.to ?? ""}
        onChange={(e) => set({ to: e.target.value })}
        InputLabelProps={{ shrink: true }}
        sx={{ width: 170 }}
      />
      {showPRID && (
        <TextField
          size="small"
          label="PR id"
          type="number"
          value={filters.pr_id ?? ""}
          onChange={(e) => set({ pr_id: e.target.value })}
          sx={{ width: 110 }}
        />
      )}
      <Button
        variant="outlined"
        color="inherit"
        size="small"
        onClick={() => onChange({})}
        disabled={isEmpty}
      >
        Clear
      </Button>
    </Stack>
  );
}

function ProcessSection({ actions }: { actions: string[] }) {
  const [draft, setDraft] = useState<EventFilters>({});
  const filters = useDebounced(draft);
  const { data, isLoading, error, isFetching } = useProcessEvents(filters);

  return (
    <>
      <FilterBar filters={draft} onChange={setDraft} actions={actions} showPRID />
      <EventCount rows={data?.length} fetching={isFetching} />
      {isLoading && <CircularProgress size={24} />}
      {error && <Alert severity="error">Failed to load process events.</Alert>}
      {data && (
        <TableContainer component={Paper} variant="outlined">
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell>When</TableCell>
                <TableCell>Action</TableCell>
                <TableCell>PR</TableCell>
                <TableCell>Actor</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {data.length === 0 && <EmptyRow cols={4} />}
              {data.map((e: ProcessEvent) => (
                <TableRow key={e.id}>
                  <TableCell sx={{ whiteSpace: "nowrap", verticalAlign: "top" }}>
                    {formatWhen(e.created_at)}
                  </TableCell>
                  <TableCell sx={{ verticalAlign: "top" }}>
                    <ActionCell action={e.action} qualifier={e.qualifier} />
                  </TableCell>
                  <TableCell sx={{ verticalAlign: "top" }}>
                    <Typography
                      component={RouterLink}
                      to={`/requests/${e.purchase_request_id}`}
                      variant="body2"
                      sx={{ color: "primary.main", textDecoration: "none" }}
                    >
                      #{e.purchase_request_id}
                    </Typography>
                  </TableCell>
                  <TableCell sx={{ verticalAlign: "top" }}>
                    <ActorCell email={e.actor_email} name={e.actor_name} />
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}
    </>
  );
}

function SystemSection({ actions }: { actions: string[] }) {
  const [draft, setDraft] = useState<EventFilters>({});
  const filters = useDebounced(draft);
  const { data, isLoading, error, isFetching } = useAuditEvents(filters);

  return (
    <>
      <FilterBar filters={draft} onChange={setDraft} actions={actions} showPRID={false} />
      <EventCount rows={data?.length} fetching={isFetching} />
      {isLoading && <CircularProgress size={24} />}
      {error && <Alert severity="error">Failed to load system events.</Alert>}
      {data && (
        <TableContainer component={Paper} variant="outlined">
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell>When</TableCell>
                <TableCell>Action</TableCell>
                <TableCell>Entity</TableCell>
                <TableCell>Detail</TableCell>
                <TableCell>Actor</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {data.length === 0 && <EmptyRow cols={5} />}
              {data.map((e: AuditEvent) => (
                <TableRow key={e.id}>
                  <TableCell sx={{ whiteSpace: "nowrap", verticalAlign: "top" }}>
                    {formatWhen(e.created_at)}
                  </TableCell>
                  <TableCell sx={{ verticalAlign: "top" }}>
                    <ActionCell action={e.action} qualifier={e.qualifier} />
                  </TableCell>
                  <TableCell sx={{ verticalAlign: "top" }}>
                    <Typography variant="body2">
                      {humanize(e.entity_type)}
                      {e.entity_id != null ? ` #${e.entity_id}` : ""}
                    </Typography>
                  </TableCell>
                  <TableCell sx={{ verticalAlign: "top" }}>
                    <Typography variant="body2" color="text.secondary">
                      {e.detail || "—"}
                    </Typography>
                  </TableCell>
                  <TableCell sx={{ verticalAlign: "top" }}>
                    <ActorCell email={e.actor_email} name={e.actor_name} />
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}
    </>
  );
}

function EmptyRow({ cols }: { cols: number }) {
  return (
    <TableRow>
      <TableCell colSpan={cols} align="center" sx={{ py: 4, color: "text.secondary" }}>
        No events match.
      </TableCell>
    </TableRow>
  );
}

// EventCount notes how many rows are shown; the backend caps a page at 2000
// newest-first, so a full page is a hint to narrow the filters.
function EventCount({ rows, fetching }: { rows?: number; fetching: boolean }) {
  if (rows == null) return null;
  return (
    <Typography variant="caption" color="text.secondary" sx={{ display: "block", mb: 1 }}>
      {rows === 2000 ? "Showing the latest 2000 events — narrow the filters to see more." : `${rows} event${rows === 1 ? "" : "s"}`}
      {fetching ? " · updating…" : ""}
    </Typography>
  );
}

export function AuditEventsPage() {
  const canView = useCanViewAuditLog();
  const [section, setSection] = useState<Section>("process");
  const { data: actions } = useEventActions(canView);

  if (!canView) {
    return (
      <Box sx={{ maxWidth: 1200, mx: "auto", p: { xs: 2, md: 4 } }}>
        <Card variant="outlined" sx={{ borderStyle: "dashed" }}>
          <CardContent sx={{ p: 4, textAlign: "center" }}>
            <Typography color="text.secondary">
              You need the admin or procurement admin role to view the audit log.
            </Typography>
          </CardContent>
        </Card>
      </Box>
    );
  }

  return (
    <Box sx={{ maxWidth: 1200, mx: "auto", p: { xs: 2, md: 4 } }}>
      <Typography variant="h5" sx={{ fontWeight: 600 }}>
        Audit log
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5, mb: 3 }}>
        An append-only record of who did what. <strong>Process events</strong> are the
        business-process tasks on purchase requests; <strong>system events</strong> are
        master-data and admin changes. Filter by actor, action, date range{" "}
        {section === "process" ? "and PR id" : ""}.
      </Typography>

      {/* Segmented control (the UI kit has no Tabs). */}
      <Stack direction="row" spacing={1} sx={{ mb: 3 }}>
        <Button
          variant={section === "process" ? "contained" : "outlined"}
          color={section === "process" ? "primary" : "inherit"}
          onClick={() => setSection("process")}
        >
          Process events
        </Button>
        <Button
          variant={section === "system" ? "contained" : "outlined"}
          color={section === "system" ? "primary" : "inherit"}
          onClick={() => setSection("system")}
        >
          System events
        </Button>
      </Stack>

      {section === "process" ? (
        <ProcessSection actions={actions?.process ?? []} />
      ) : (
        <SystemSection actions={actions?.audit ?? []} />
      )}
    </Box>
  );
}
