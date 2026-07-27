import { Box, Card, CardContent, Stack, Typography } from "@wso2/oxygen-ui";
import { useProcurementAccess } from "../hooks/useProcurementAccess";
import { useRelatedDocuments } from "../hooks/usePurchaseRequests";
import { RecordRow } from "./RecordCard";
import { relatedGroups } from "../lib/caseGraph";
import type { CaseCurrent } from "../lib/caseGraph";

// RelatedDocuments shows the records in the same case that are NOT the current
// record's direct parent or direct children — its indirect ancestors, deeper
// descendants and siblings. (Direct neighbours live in the main content.)
// Procurement-access only — renders nothing otherwise.
export function RelatedDocuments({ prId, current }: { prId: number; current: CaseCurrent }) {
  const procurement = useProcurementAccess();
  const { data, isLoading, error } = useRelatedDocuments(prId, procurement);

  if (!procurement) return null;

  const groups = data ? relatedGroups(data, current) : [];

  return (
    <Card variant="outlined" sx={{ mt: 3 }}>
      <CardContent>
        <Typography variant="h6" sx={{ mb: 1.5, fontWeight: 600 }}>
          Related documents
        </Typography>
        {error ? (
          <Typography variant="body2" color="error.main">
            Failed to load related documents.
          </Typography>
        ) : isLoading || !data ? (
          <Typography variant="body2" color="text.secondary">
            Loading…
          </Typography>
        ) : groups.length === 0 ? (
          <Typography variant="body2" color="text.secondary">
            No related documents yet.
          </Typography>
        ) : (
          <Stack spacing={2}>
            {groups.map((group) => (
              <Box key={group.label} sx={{ display: { sm: "flex" }, gap: 2 }}>
                <Typography
                  variant="body2"
                  color="text.secondary"
                  sx={{ width: 176, flexShrink: 0, mb: { xs: 0.5, sm: 0 }, fontWeight: 600 }}
                >
                  {group.label}
                </Typography>
                <Stack spacing={0.75} sx={{ flex: 1 }}>
                  {group.records.map((r) => (
                    <RecordRow key={`${r.kind}-${r.id}`} record={r} />
                  ))}
                </Stack>
              </Box>
            ))}
          </Stack>
        )}
      </CardContent>
    </Card>
  );
}
