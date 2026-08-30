import { Box, Card, CardContent, Typography } from "@wso2/oxygen-ui";

// AnalyticsAccessDenied is the placeholder both Analytics pages render when the
// viewer lacks the role. UX-only — the backend enforces the same rule
// (AnalyticsHandler.requireAccess), so this only avoids a bare 403.
export function AnalyticsAccessDenied() {
  return (
    <Box sx={{ maxWidth: 1200, mx: "auto", p: { xs: 2, md: 4 } }}>
      <Card variant="outlined" sx={{ borderStyle: "dashed" }}>
        <CardContent sx={{ p: 4, textAlign: "center" }}>
          <Typography color="text.secondary">
            You need the admin or procurement admin role to view analytics.
          </Typography>
        </CardContent>
      </Card>
    </Box>
  );
}
