import { Link } from "react-router-dom";
import {
  Alert,
  Box,
  Card,
  CardActionArea,
  CardContent,
  CircularProgress,
  Divider,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { useHome } from "../hooks/useHome";
import { useMe } from "../hooks/useMe";
import {
  activityLabel,
  relativeTime,
  type ApprovalsHome,
  type HomeActivity,
  type ProcurementHome,
  type StaffHome,
} from "../types/api";

// HomePage is the role-based landing page. It renders one section per role group
// the caller belongs to (staff / approver / procurement), each a row of count
// tiles plus a "Latest activity" feed. The backend decides which blocks apply.
export function HomePage() {
  const { data: me } = useMe();
  const { data, isLoading, error } = useHome();

  const greeting = me?.name?.split(" ")[0] || me?.email?.split("@")[0] || "there";

  return (
    <Box sx={{ maxWidth: 1200, mx: "auto", p: { xs: 2, md: 4 } }}>
      <Box sx={{ mb: 4 }}>
        <Typography variant="h4" sx={{ fontWeight: 600, letterSpacing: "-0.01em" }}>
          Welcome back, {greeting}
        </Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
          A snapshot of what's relevant to you across the purchasing process.
        </Typography>
      </Box>

      {isLoading && <CircularProgress size={24} />}
      {!!error && <Alert severity="error">Failed to load your home page.</Alert>}

      {data && (
        <Stack spacing={5}>
          {data.staff && <StaffSection block={data.staff} />}
          {data.approvals && <ApprovalsSection block={data.approvals} />}
          {data.procurement && <ProcurementSection block={data.procurement} />}
        </Stack>
      )}
    </Box>
  );
}

function StaffSection({ block }: { block: StaffHome }) {
  return (
    <Section title="My requests" subtitle="Requests you've submitted.">
      <TileRow>
        <StatTile to="/my-requests" label="My requests" value={block.my_requests_count} accent="primary" />
        <StatTile to="/my-requests" label="Completed" value={block.completed_count} accent="success" />
      </TileRow>
      <ActivityCard
        title="Latest activity on my requests"
        items={block.recent_activity}
        empty="No activity on your requests yet."
      />
    </Section>
  );
}

function ApprovalsSection({ block }: { block: ApprovalsHome }) {
  return (
    <Section title="Approvals" subtitle="Requests awaiting or decided by you.">
      <TileRow>
        <StatTile to="/approvals" label="Pending approvals" value={block.pending_count} accent="warning" />
        <StatTile to="/approvals" label="Completed approvals" value={block.completed_count} accent="success" />
      </TileRow>
      <ActivityCard
        title="Latest activity on pending approvals"
        items={block.recent_activity}
        empty="Nothing awaiting your approval."
      />
    </Section>
  );
}

function ProcurementSection({ block }: { block: ProcurementHome }) {
  return (
    <Section title="Procurement" subtitle="The team's purchasing queue.">
      <TileRow>
        <StatTile to="/requests" label="Pending requests" value={block.pending_count} accent="primary" />
        <StatTile
          to="/requests"
          label="Awaiting delivery"
          value={block.awaiting_delivery_count}
          accent="warning"
        />
        <StatTile to="/requests" label="Completed" value={block.completed_count} accent="success" />
      </TileRow>
      <ActivityCard
        title="Latest activity on the queue"
        items={block.recent_activity}
        empty="No recent procurement activity."
      />
    </Section>
  );
}

function Section({
  title,
  subtitle,
  children,
}: {
  title: string;
  subtitle: string;
  children: React.ReactNode;
}) {
  return (
    <Box component="section">
      <Box sx={{ mb: 1.5 }}>
        <Typography variant="h6" sx={{ fontWeight: 600, letterSpacing: "-0.01em" }}>
          {title}
        </Typography>
        <Typography variant="body2" color="text.secondary">
          {subtitle}
        </Typography>
      </Box>
      {children}
    </Box>
  );
}

function TileRow({ children }: { children: React.ReactNode }) {
  return (
    <Box
      sx={{
        display: "grid",
        gap: 2,
        gridTemplateColumns: {
          xs: "repeat(2, 1fr)",
          sm: "repeat(3, 1fr)",
          lg: "repeat(4, 1fr)",
        },
      }}
    >
      {children}
    </Box>
  );
}

type Accent = "primary" | "warning" | "success";

function StatTile({
  to,
  label,
  value,
  accent,
}: {
  to: string;
  label: string;
  value: number;
  accent: Accent;
}) {
  return (
    <Card variant="outlined">
      <CardActionArea component={Link} to={to} sx={{ height: "100%" }}>
        <CardContent sx={{ display: "flex", flexDirection: "column", gap: 0.5 }}>
          <Typography
            variant="h4"
            sx={{ fontWeight: 600, color: `${accent}.main`, fontVariantNumeric: "tabular-nums" }}
          >
            {value}
          </Typography>
          <Typography variant="body2" sx={{ fontWeight: 500 }} color="text.secondary">
            {label}
          </Typography>
        </CardContent>
      </CardActionArea>
    </Card>
  );
}

function ActivityCard({
  title,
  items,
  empty,
}: {
  title: string;
  items: HomeActivity[];
  empty: string;
}) {
  return (
    <Card variant="outlined" sx={{ mt: 2 }}>
      <CardContent>
        <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1.5 }}>
          {title}
        </Typography>
        {items.length === 0 ? (
          <Typography variant="body2" color="text.secondary">
            {empty}
          </Typography>
        ) : (
          <Box>
            {items.map((a, i) => (
              <Box key={i}>
                {i > 0 && <Divider />}
                <Box
                  component={Link}
                  to={`/requests/${a.purchase_request_id}`}
                  sx={{
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                    gap: 1.5,
                    py: 1.25,
                    textDecoration: "none",
                    color: "inherit",
                    "&:hover": { bgcolor: "action.hover" },
                  }}
                >
                  <Box sx={{ minWidth: 0 }}>
                    <Typography component="span" variant="body2" sx={{ fontWeight: 500 }}>
                      {activityLabel(a.action, a.qualifier)}
                    </Typography>
                    <Typography component="span" variant="caption" color="primary.main" sx={{ ml: 1 }}>
                      {a.reference || `#${a.purchase_request_id}`}
                    </Typography>
                    {a.title && (
                      <Typography
                        component="span"
                        variant="caption"
                        color="text.secondary"
                        sx={{ ml: 0.5 }}
                        noWrap
                      >
                        — {a.title}
                      </Typography>
                    )}
                  </Box>
                  <Stack
                    direction="row"
                    spacing={1}
                    sx={{ flexShrink: 0, alignItems: "center", color: "text.secondary" }}
                  >
                    <Typography
                      component="span"
                      variant="caption"
                      sx={{ display: { xs: "none", sm: "inline" } }}
                    >
                      {a.actor_email}
                    </Typography>
                    <Typography
                      component="span"
                      variant="caption"
                      title={new Date(a.created_at).toLocaleString()}
                    >
                      {relativeTime(a.created_at)}
                    </Typography>
                  </Stack>
                </Box>
              </Box>
            ))}
          </Box>
        )}
      </CardContent>
    </Card>
  );
}
