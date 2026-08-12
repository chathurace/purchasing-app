// caseGraph derives the navigable structure of a single procurement case (one
// purchase request and its quotations and contracts) from the flat
// RelatedDocuments bundle. The chain is PR → Quotation → Contract; each record
// has exactly one parent, so the upstream lineage is unambiguous while
// downstream branches. All three views (chain stepper, direct parent/children,
// related entities) are computed from this one payload — no extra fetches.
import {
  conRef,
  formatMoney,
  grnRef,
  invRef,
  prReference,
  quoRef,
} from "../types/api";
import type {
  Contract,
  ContractStatus,
  GRN,
  Invoice,
  InvoiceStatus,
  PRStatus,
  Quotation,
  QuotationStatus,
  RelatedDocuments,
} from "../types/api";

export type CaseKind = "pr" | "quotation" | "contract" | "grn" | "invoice";
export type CaseStatus = PRStatus | QuotationStatus | ContractStatus | InvoiceStatus;

export interface CaseCurrent {
  kind: CaseKind;
  id: number;
}

// CaseRecord is a uniform, render-ready reference to any record in the case.
export interface CaseRecord {
  kind: CaseKind;
  id: number;
  ref: string;
  to: string;
  detail?: string;
  status?: CaseStatus; // omitted for records without a status (e.g. GRNs)
  highlight?: boolean; // on the "winning" path (selected quotation / signed contract)
}

export interface RecordGroup {
  label: string;
  records: CaseRecord[];
}

// --- record builders ---

function vendorLine(r: Quotation | Contract): string {
  const name = r.vendor?.name ?? `Vendor #${r.vendor_id}`;
  return `${name} · ${formatMoney(r.total_amount, r.currency)}`;
}

function prRecord(data: RelatedDocuments): CaseRecord {
  const pr = data.purchase_request;
  return { kind: "pr", id: pr.id, ref: prReference(pr), to: `/requests/${pr.id}`, detail: pr.title, status: pr.status };
}

function quoRecord(q: Quotation): CaseRecord {
  return {
    kind: "quotation",
    id: q.id,
    ref: quoRef(q.id),
    to: `/quotations/${q.id}`,
    detail: vendorLine(q),
    status: q.status,
    highlight: q.status === "selected",
  };
}

function conRecord(c: Contract): CaseRecord {
  return {
    kind: "contract",
    id: c.id,
    ref: conRef(c.id),
    to: `/contracts/${c.id}`,
    detail: vendorLine(c),
    status: c.status,
    highlight: c.status === "signed",
  };
}

function grnRecord(g: GRN): CaseRecord {
  const vendor = g.vendor?.name ?? `Vendor #${g.vendor_id}`;
  return {
    kind: "grn",
    id: g.id,
    ref: grnRef(g.id),
    to: `/grns/${g.id}`,
    detail: `${vendor} · received ${g.received_date}`,
  };
}

function invRecord(inv: Invoice): CaseRecord {
  const vendor = inv.vendor?.name ?? `Vendor #${inv.vendor_id}`;
  return {
    kind: "invoice",
    id: inv.id,
    ref: invRef(inv.id),
    to: `/invoices/${inv.id}`,
    detail: `${vendor} · ${formatMoney(inv.total_amount, inv.currency)}`,
    status: inv.status,
  };
}

// --- lineage helpers ---

// contractLineage walks up from a contract to its quotation. The link may be
// absent (e.g. a contract drafted with no quotation), so it is nullable.
function contractLineage(data: RelatedDocuments, contractId: number | null) {
  const contract = contractId != null ? data.contracts.find((c) => c.id === contractId) : undefined;
  const quotation =
    contract && contract.quotation_id != null
      ? data.quotations.find((q) => q.id === contract.quotation_id)
      : undefined;
  return { contract, quotation };
}

// contractIdOf returns the contract a GRN or invoice is recorded against.
function contractIdOf(data: RelatedDocuments, current: CaseCurrent): number | null {
  if (current.kind === "grn") return data.grns.find((g) => g.id === current.id)?.contract_id ?? null;
  if (current.kind === "invoice") return data.invoices.find((i) => i.id === current.id)?.contract_id ?? null;
  return null;
}

// --- chain stepper ---

export type StepState = "ancestor" | "current" | "descendant";

export interface StepView {
  kind: CaseKind;
  label: string;
  state: StepState;
  ref?: string; // ancestor / current
  to?: string; // ancestor (clickable)
  count?: number; // descendant (records in the current record's subtree)
}

// ORDER is the linear upstream chain. GRNs and invoices are not part of it — they
// are parallel leaves off a contract, appended individually by buildSteps.
const ORDER: CaseKind[] = ["pr", "quotation", "contract"];
const STEP_LABELS: Record<CaseKind, string> = {
  pr: "Request",
  quotation: "Quotation",
  contract: "Contract",
  grn: "GRN",
  invoice: "Invoice",
};

// resolveLineage walks up from the current record, returning the specific
// quotation / contract ids on its path (a GRN/invoice's lineage is reached via
// its contract).
function resolveLineage(data: RelatedDocuments, current: CaseCurrent) {
  let quoId: number | null = null;
  let conId: number | null = null;

  if (current.kind === "quotation") {
    quoId = current.id;
  } else if (current.kind === "contract") {
    conId = current.id;
    quoId = contractLineage(data, current.id).quotation?.id ?? null;
  } else if (current.kind === "grn" || current.kind === "invoice") {
    conId = contractIdOf(data, current);
    quoId = contractLineage(data, conId).quotation?.id ?? null;
  }
  return { quoId, conId };
}

// subtreeCounts counts the descendants at each stage that hang off the current
// record specifically (not the whole case), so the stepper reflects "this" path.
function subtreeCounts(
  data: RelatedDocuments,
  current: CaseCurrent,
): Record<CaseKind, number> {
  const counts: Record<CaseKind, number> = { pr: 0, quotation: 0, contract: 0, grn: 0, invoice: 0 };
  if (current.kind === "pr") {
    counts.quotation = data.quotations.length;
    counts.contract = data.contracts.length;
  } else if (current.kind === "quotation") {
    counts.contract = data.contracts.filter((c) => c.quotation_id === current.id).length;
  }
  return counts;
}

export function buildSteps(data: RelatedDocuments, current: CaseCurrent): StepView[] {
  // GRNs and invoices are leaves: the three base stages all sit above them, so
  // the current index is past the end of ORDER and a 4th node is appended below.
  const leaf = current.kind === "grn" || current.kind === "invoice" ? current.kind : null;
  const curIdx = leaf ? ORDER.length : ORDER.indexOf(current.kind);
  const { quoId, conId } = resolveLineage(data, current);
  const counts = subtreeCounts(data, current);

  const refs: Partial<Record<CaseKind, { ref: string; to: string }>> = {
    pr: { ref: prReference(data.purchase_request), to: `/requests/${data.purchase_request.id}` },
  };
  if (quoId != null) refs.quotation = { ref: quoRef(quoId), to: `/quotations/${quoId}` };
  if (conId != null) refs.contract = { ref: conRef(conId), to: `/contracts/${conId}` };

  const steps: StepView[] = ORDER.map((kind, idx) => {
    const label = STEP_LABELS[kind];
    if (idx < curIdx) return { kind, label, state: "ancestor", ref: refs[kind]?.ref, to: refs[kind]?.to };
    if (idx === curIdx) return { kind, label, state: "current", ref: refs[kind]?.ref };
    return { kind, label, state: "descendant", count: counts[kind] };
  });

  if (leaf) {
    const ref = leaf === "grn" ? grnRef(current.id) : invRef(current.id);
    steps.push({ kind: leaf, label: STEP_LABELS[leaf], state: "current", ref });
  }
  return steps;
}

// --- direct parent (shown in main content) ---

export interface DirectParent {
  label: string;
  record: CaseRecord;
}

export function directParent(data: RelatedDocuments, current: CaseCurrent): DirectParent | null {
  if (current.kind === "quotation") {
    return { label: "For purchase request", record: prRecord(data) };
  }
  if (current.kind === "contract") {
    const c = data.contracts.find((x) => x.id === current.id);
    const q = c && c.quotation_id != null ? data.quotations.find((x) => x.id === c.quotation_id) : undefined;
    if (q) return { label: "Based on quotation", record: quoRecord(q) };
    // A contract with no quotation falls back to the purchase request as parent.
    return { label: "For purchase request", record: prRecord(data) };
  }
  if (current.kind === "grn" || current.kind === "invoice") {
    // A GRN/invoice is recorded against a signed contract — its direct parent.
    const con = data.contracts.find((c) => c.id === contractIdOf(data, current));
    const label = current.kind === "grn" ? "Received against" : "Billed against";
    if (con) return { label, record: conRecord(con) };
    return { label: "For purchase request", record: prRecord(data) };
  }
  return null; // a purchase request has no parent
}

// --- related entities (indirect ancestors, deeper descendants, siblings) ---

// relatedGroups lists the case records worth cross-linking from the current
// page. `exclude` drops records the page already renders in full elsewhere (the
// PR page, for instance, shows the recommendation's contract in its own card),
// so the card never repeats what is already on screen.
export function relatedGroups(
  data: RelatedDocuments,
  current: CaseCurrent,
  exclude: CaseCurrent[] = [],
): RecordGroup[] {
  const groups: RecordGroup[] = [];
  const excluded = new Set(exclude.map((e) => `${e.kind}-${e.id}`));
  const pushGroup = (label: string, records: CaseRecord[]) => {
    const kept = records.filter((r) => !excluded.has(`${r.kind}-${r.id}`));
    if (kept.length > 0) groups.push({ label, records: kept });
  };

  // grnsForContracts / invoicesForContracts collect the fulfillment records that
  // belong to a set of contracts — used to surface a case's GRNs/invoices on the
  // upstream (quotation) page whose subtree those contracts sit in.
  const grnsForContracts = (ids: Set<number>) => data.grns.filter((g) => ids.has(g.contract_id)).map(grnRecord);
  const invoicesForContracts = (ids: Set<number>) =>
    data.invoices.filter((i) => ids.has(i.contract_id)).map(invRecord);

  if (current.kind === "pr") {
    // Quotations are the direct children (shown inline); everything below is indirect.
    pushGroup("Contracts", data.contracts.map(conRecord));
    pushGroup("Goods received (GRNs)", data.grns.map(grnRecord));
    pushGroup("Invoices", data.invoices.map(invRecord));
    return groups;
  }

  if (current.kind === "quotation") {
    pushGroup("Purchase request", [prRecord(data)]);
    pushGroup(
      "Other quotations for this request",
      data.quotations.filter((x) => x.id !== current.id).map(quoRecord),
    );
    const myConIds = new Set(data.contracts.filter((c) => c.quotation_id === current.id).map((c) => c.id));
    pushGroup("Resulting contracts", data.contracts.filter((c) => myConIds.has(c.id)).map(conRecord));
    pushGroup("Goods received (GRNs)", grnsForContracts(myConIds));
    pushGroup("Invoices", invoicesForContracts(myConIds));
    return groups;
  }

  if (current.kind === "grn" || current.kind === "invoice") {
    // The contract is the direct parent (shown in main content); the chain above
    // it is indirect, and the other fulfillment records on the same contract are
    // siblings. GRNs and invoices on the contract page itself stay in the
    // fulfillment section, so they are only cross-linked from here.
    const conId = contractIdOf(data, current);
    const { quotation } = contractLineage(data, conId);
    pushGroup("Purchase request", [prRecord(data)]);
    if (quotation) pushGroup("Quotation", [quoRecord(quotation)]);
    const grns = data.grns.filter((g) => g.contract_id === conId);
    const invoices = data.invoices.filter((i) => i.contract_id === conId);
    if (current.kind === "grn") {
      pushGroup("Other goods received notes for this contract", grns.filter((g) => g.id !== current.id).map(grnRecord));
      pushGroup("Invoices for this contract", invoices.map(invRecord));
    } else {
      pushGroup("Goods received notes for this contract", grns.map(grnRecord));
      pushGroup("Other invoices for this contract", invoices.filter((i) => i.id !== current.id).map(invRecord));
    }
    return groups;
  }

  // contract: the quotation is its direct parent (shown in main content); the PR
  // above it is indirect. When there is no quotation, the PR is the direct parent
  // instead, so it is omitted here. GRNs/invoices live in the page's fulfillment
  // section, so they are not duplicated here.
  const c = data.contracts.find((x) => x.id === current.id);
  const q = c && c.quotation_id != null ? data.quotations.find((x) => x.id === c.quotation_id) : undefined;
  if (q) pushGroup("Purchase request", [prRecord(data)]);
  pushGroup("Other contracts for this request", data.contracts.filter((x) => x.id !== current.id).map(conRecord));
  return groups;
}
