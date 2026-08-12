import { Box, Card, CardContent, Stack, Typography } from "@wso2/oxygen-ui";
import { useProcurementAccess } from "../hooks/useProcurementAccess";
import { useRelatedDocuments } from "../hooks/usePurchaseRequests";
import { RecordRow } from "./RecordCard";
import { relatedGroups } from "../lib/caseGraph";
import type { CaseCurrent } from "../lib/caseGraph";

// RelatedEntities cross-links the records in the same case that the current page
// does not already render — its indirect ancestors, deeper descendants and
// siblings. (Direct neighbours live in the main content; pass `exclude` for
// anything else the page shows in a card of its own, e.g. the PR page's
// recommendation contract.) Procurement-access only, and it renders nothing at
// all unless there is at least one record to link to.
export function RelatedEntities({
  prId,
  current,
  exclude,
}: {
  prId: number;
  current: CaseCurrent;
  exclude?: CaseCurrent[];
}) {
  const procurement = useProcurementAccess();
  const { data, error } = useRelatedDocuments(prId, procurement);

  if (!procurement) return null;

  const groups = data ? relatedGroups(data, current, exclude) : [];

  // Nothing to link to (still loading, or the case has no other records): stay
  // out of the page entirely rather than showing an empty card.
  if (groups.length === 0) {
    if (!error) return null;
    return (
      <Card variant="outlined" sx={{ mt: 3 }}>
        <CardContent>
          <Typography variant="h6" sx={{ mb: 1.5, fontWeight: 600 }}>
            Related entities
          </Typography>
          <Typography variant="body2" color="error.main">
            Failed to load related entities.
          </Typography>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card variant="outlined" sx={{ mt: 3 }}>
      <CardContent>
        <Typography variant="h6" sx={{ mb: 1.5, fontWeight: 600 }}>
          Related entities
        </Typography>
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
      </CardContent>
    </Card>
  );
}
