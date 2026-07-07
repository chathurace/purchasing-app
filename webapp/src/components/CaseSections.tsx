import { useFinanceAccess } from "../hooks/useFinanceAccess";
import { useRelatedDocuments } from "../hooks/usePurchaseRequests";
import { RecordCard } from "./RecordCard";
import { contractsForQuotation, directParent } from "../lib/caseGraph";
import type { CaseCurrent } from "../lib/caseGraph";

// DirectParentCard shows the record the current one was created from (its
// immediate upstream link) as a main-content section. Finance-access only.
export function DirectParentCard({ prId, current }: { prId: number; current: CaseCurrent }) {
  const finance = useFinanceAccess();
  const { data } = useRelatedDocuments(prId, finance);
  if (!finance || !data) return null;

  const parent = directParent(data, current);
  if (!parent) return null;
  return <RecordCard title={parent.label} records={[parent.record]} />;
}

// ResultingContracts shows the contracts drafted from a quotation (its direct
// downstream link), with a launcher to draft another. Finance-access only.
export function ResultingContracts({ prId, quotationId }: { prId: number; quotationId: number }) {
  const finance = useFinanceAccess();
  const { data } = useRelatedDocuments(prId, finance);
  if (!finance) return null;

  const contracts = data ? contractsForQuotation(data, quotationId) : [];
  return (
    <RecordCard
      title="Resulting contracts"
      action={{ to: `/quotations/${quotationId}/contracts/new`, label: "+ Create contract" }}
      actionProminent
      records={contracts}
      emptyCta={{
        title: "Create a contract",
        subtitle: "Draft a contract from this quotation to move toward signing.",
      }}
      empty="No contracts drafted from this quotation yet."
    />
  );
}
