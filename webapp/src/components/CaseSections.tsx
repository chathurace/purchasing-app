import { useProcurementAccess } from "../hooks/useProcurementAccess";
import { useRelatedDocuments } from "../hooks/usePurchaseRequests";
import { RecordCard } from "./RecordCard";
import { directParent } from "../lib/caseGraph";
import type { CaseCurrent } from "../lib/caseGraph";

// DirectParentCard shows the record the current one was created from (its
// immediate upstream link) as a main-content section. Procurement-access only.
export function DirectParentCard({ prId, current }: { prId: number; current: CaseCurrent }) {
  const procurement = useProcurementAccess();
  const { data } = useRelatedDocuments(prId, procurement);
  if (!procurement || !data) return null;

  const parent = directParent(data, current);
  if (!parent) return null;
  return <RecordCard title={parent.label} records={[parent.record]} />;
}
