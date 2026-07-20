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
    <div className="mt-6 rounded border bg-white p-6">
      <h2 className="mb-3 font-medium text-gray-900">Related documents</h2>
      {error ? (
        <p className="text-sm text-red-600">Failed to load related documents.</p>
      ) : isLoading || !data ? (
        <p className="text-sm text-gray-400">Loading…</p>
      ) : groups.length === 0 ? (
        <p className="text-sm text-gray-400">No related documents yet.</p>
      ) : (
        <dl className="space-y-4 text-sm">
          {groups.map((group) => (
            <div key={group.label} className="sm:flex sm:gap-4">
              <dt className="mb-1 w-44 shrink-0 font-medium text-gray-500 sm:mb-0">{group.label}</dt>
              <dd className="flex-1 space-y-1.5">
                {group.records.map((r) => (
                  <RecordRow key={`${r.kind}-${r.id}`} record={r} />
                ))}
              </dd>
            </div>
          ))}
        </dl>
      )}
    </div>
  );
}
