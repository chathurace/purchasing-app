import { useMemo, useRef, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { usePurchaseRequest } from "../hooks/usePurchaseRequests";
import { useBudgetUnitLookup, useBudgetUnitApprovers } from "../hooks/useBudgetUnits";
import { useMe } from "../hooks/useMe";
import { useProcurementAccess } from "../hooks/useProcurementAccess";
import { useQuotationsForPR, useQuotation } from "../hooks/useQuotations";
import { StatusBadge } from "../components/StatusBadge";
import { EntityStatusBadge } from "../components/EntityStatusBadge";
import { RequisitionForm, requisitionTitle } from "../components/RequisitionForm";
import { TeamLeadApprovalCard } from "../components/TeamLeadApprovalCard";
import { AssignmentCard } from "../components/AssignmentCard";
import { RelatedDocuments } from "../components/RelatedDocuments";
import { ChainStepper } from "../components/ChainStepper";
import { VendorSelect } from "../components/VendorSelect";
import { RecommendationSection } from "../components/RecommendationSection";
import {
  deleteDocument,
  downloadDocument,
  rejectPurchaseRequest,
  updatePurchaseRequest,
  uploadDocument,
} from "../api/purchaseRequests";
import {
  createQuotation,
  downloadQuotationDocument,
  uploadQuotationPDF,
} from "../api/quotations";
import { ApiError } from "../api/client";
import { EDITABLE_STATUSES, formatMoney, prReference, quoRef } from "../types/api";
import type {
  Document,
  Me,
  PurchaseRequest,
  PurchaseRequestInput,
  Quotation,
} from "../types/api";

function toInput(pr: PurchaseRequest): PurchaseRequestInput {
  return {
    title: pr.title,
    budget_unit_id: pr.budget_unit_id,
    comments: pr.comments,
    items: [],
    links: [],
    team: pr.team,
    entity: pr.entity,
    category: pr.category,
    estimated_value: pr.estimated_value,
    currency: pr.currency,
    budget_approver_name: pr.budget_approver_name ?? "",
    budget_approver_email: pr.budget_approver_email ?? "",
    team_lead_email: pr.team_lead_email ?? "",
    details: pr.details ?? {},
  };
}

export function PurchaseRequestDetailPage() {
  const { id } = useParams();
  const prId = Number(id);
  const qc = useQueryClient();
  const { data: pr, isLoading, error } = usePurchaseRequest(prId);
  const { data: me } = useMe();
  const procurement = useProcurementAccess();

  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState<PurchaseRequestInput | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const fileRef = useRef<HTMLInputElement>(null);

  const canEdit = useMemo(() => {
    if (!pr || !me) return false;
    return me.id === pr.requester_id && EDITABLE_STATUSES.includes(pr.status);
  }, [pr, me]);

  // Enter edit mode with a fresh draft seeded from the current PR. Seeding lives
  // in the Edit click (below) rather than an effect keyed on `pr`: re-deriving
  // the draft whenever the PR query re-references (a background refetch, a
  // sibling card's invalidation) would silently discard the user's in-progress
  // edits — e.g. estimated value / currency, which also trigger an approver
  // re-fetch on each keystroke.
  const startEditing = () => {
    if (pr) setDraft(toInput(pr));
    setEditing(true);
  };

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ["purchase-requests", prId] });
    qc.invalidateQueries({ queryKey: ["purchase-requests"] });
  };

  const saveMutation = useMutation({
    mutationFn: () => updatePurchaseRequest(prId, { ...draft!, title: requisitionTitle(draft!) }),
    onSuccess: () => {
      invalidate();
      setEditing(false);
      setActionError(null);
    },
    onError: (e) => setActionError(e instanceof ApiError ? e.message : "Failed to save"),
  });

  const uploadMutation = useMutation({
    mutationFn: (file: File) => uploadDocument(prId, file),
    onSuccess: invalidate,
    onError: (e) => setActionError(e instanceof ApiError ? e.message : "Upload failed"),
  });

  const deleteMutation = useMutation({
    mutationFn: (docId: number) => deleteDocument(prId, docId),
    onSuccess: invalidate,
    onError: (e) => setActionError(e instanceof ApiError ? e.message : "Delete failed"),
  });

  if (isLoading) return <p className="text-gray-500">Loading…</p>;
  if (error || !pr) return <p className="text-red-600">Failed to load request.</p>;

  const onPickFile = (file: File | null) => {
    if (!file) return;
    if (!/\.(pdf|docx)$/i.test(file.name)) {
      setActionError(`Only .pdf and .docx files are allowed (got ${file.name}).`);
      return;
    }
    setActionError(null);
    uploadMutation.mutate(file);
    if (fileRef.current) fileRef.current.value = "";
  };

  return (
    <div className="mx-auto max-w-3xl">
      <div className="mb-4 flex items-center gap-2 text-sm text-slate-400">
        <Link to="/requests" className="text-indigo-600 hover:underline">
          Requests
        </Link>
        <span>/</span>
        <span className="text-slate-500">{prReference(pr)}</span>
      </div>

      <ChainStepper prId={pr.id} current={{ kind: "pr", id: pr.id }} />

      <div className="mb-4 flex items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight text-slate-900">
            {pr.title || prReference(pr)}
          </h1>
          <div className="mt-2">
            <StatusBadge status={pr.status} />
          </div>
        </div>
        {canEdit &&
          (editing ? (
            <button
              onClick={() => {
                setEditing(false);
                setActionError(null);
              }}
              className="btn-secondary"
            >
              Cancel editing
            </button>
          ) : (
            <button onClick={startEditing} className="btn-secondary">
              Edit details
            </button>
          ))}
      </div>

      {!canEdit && (
        <p className="mb-4 rounded-lg border border-slate-200 bg-slate-50 px-3 py-2 text-sm text-slate-600">
          This request is read-only in its current state.
        </p>
      )}

      {actionError && !editing && <p className="mb-4 text-sm text-red-600">{actionError}</p>}

      {editing && draft ? (
        <RequisitionForm
          value={draft}
          onChange={setDraft}
          onSubmit={() => saveMutation.mutate()}
          submitting={saveMutation.isPending}
          submitLabel="Save changes"
          error={actionError}
        />
      ) : (
        <div className="app-card p-6">
          <ReadOnlyView pr={pr} />
        </div>
      )}

      {/* Documents */}
      <div className="app-card mt-6 p-6">
        <div className="mb-3 flex items-center justify-between">
          <h2 className="font-semibold text-slate-900">Documents</h2>
          {canEdit && (
            <label className="cursor-pointer text-sm font-medium text-indigo-600 hover:text-indigo-700">
              + Add document
              <input
                ref={fileRef}
                type="file"
                accept=".pdf,.docx"
                className="hidden"
                onChange={(e) => onPickFile(e.target.files?.[0] ?? null)}
              />
            </label>
          )}
        </div>
        {pr.documents.length === 0 ? (
          <p className="text-sm text-gray-400">No documents attached.</p>
        ) : (
          <ul className="divide-y">
            {pr.documents.map((doc) => (
              <DocumentRow
                key={doc.id}
                prId={pr.id}
                doc={doc}
                canEdit={canEdit}
                onDelete={() => deleteMutation.mutate(doc.id)}
              />
            ))}
          </ul>
        )}
      </div>

      <TeamLeadApprovalCard pr={pr} me={me} />

      <AssignmentCard pr={pr} me={me} />

      {procurement && pr.my_can_work && <ProcurementSection pr={pr} />}

      <RecommendationSection pr={pr} />

      <RelatedDocuments prId={pr.id} current={{ kind: "pr", id: pr.id }} />
    </div>
  );
}

// QuotationRow renders a single quotation on the PR page as an expandable row.
// Collapsed it shows the reference, vendor, notes/total and status; expanded it
// loads the full quotation and shows basic info plus a link to download the
// quotation PDF (and any other documents).
function QuotationRow({ q }: { q: Quotation }) {
  const [open, setOpen] = useState(false);
  return (
    <li className="py-2 text-sm">
      <div className="flex items-start justify-between gap-3">
        <button
          type="button"
          onClick={() => setOpen((v) => !v)}
          className="flex min-w-0 items-start gap-2 text-left"
          aria-expanded={open}
        >
          <span className="mt-0.5 select-none text-gray-400">{open ? "▾" : "▸"}</span>
          <span className="min-w-0">
            <span className="font-medium text-indigo-600">
              {quoRef(q.id)} — {q.vendor?.name ?? `Vendor #${q.vendor_id}`}
            </span>
            {q.notes && <span className="mt-0.5 block truncate text-gray-500">{q.notes}</span>}
            {q.total_amount > 0 && (
              <span className="mt-0.5 block text-gray-400">{formatMoney(q.total_amount, q.currency)}</span>
            )}
          </span>
        </button>
        <EntityStatusBadge status={q.status} />
      </div>
      {open && <QuotationExpanded quotationId={q.id} />}
    </li>
  );
}

// QuotationExpanded lazily loads the full quotation (the PR-page list only
// carries summaries) to show its basic info, the primary quotation PDF and any
// other attached documents.
function QuotationExpanded({ quotationId }: { quotationId: number }) {
  const { data: q, isLoading } = useQuotation(quotationId);
  if (isLoading || !q) {
    return <p className="ml-6 mt-2 text-xs text-gray-400">Loading…</p>;
  }
  const others = q.documents ?? [];
  return (
    <div className="ml-6 mt-2 space-y-3 rounded-md border bg-gray-50 p-3 text-xs">
      <dl className="grid grid-cols-2 gap-x-4 gap-y-1">
        <div>
          <dt className="font-medium text-gray-500">Total</dt>
          <dd className="text-gray-900">{formatMoney(q.total_amount, q.currency)}</dd>
        </div>
        <div>
          <dt className="font-medium text-gray-500">Valid until</dt>
          <dd className="text-gray-900">{q.valid_until ?? "—"}</dd>
        </div>
      </dl>

      {q.notes && <p className="whitespace-pre-wrap text-gray-600">{q.notes}</p>}

      {q.items.length > 0 && (
        <ul className="list-disc pl-4 text-gray-600">
          {q.items.map((it) => (
            <li key={it.id}>
              {it.description} — qty {it.quantity} × {formatMoney(it.unit_price, q.currency)}
            </li>
          ))}
        </ul>
      )}

      <div>
        <p className="font-medium text-gray-500">Quotation PDF</p>
        {q.quotation_document ? (
          <button
            type="button"
            onClick={() => downloadQuotationDocument(q.id, q.quotation_document!)}
            className="text-indigo-600 hover:underline"
          >
            📄 {q.quotation_document.filename}
          </button>
        ) : (
          <p className="text-gray-400">None attached.</p>
        )}
      </div>

      {others.length > 0 && (
        <div>
          <p className="font-medium text-gray-500">Other documents</p>
          <ul className="space-y-0.5">
            {others.map((doc) => (
              <li key={doc.id}>
                <button
                  type="button"
                  onClick={() => downloadQuotationDocument(q.id, doc)}
                  className="text-indigo-600 hover:underline"
                >
                  📎 {doc.filename}
                </button>
              </li>
            ))}
          </ul>
        </div>
      )}

      <Link to={`/quotations/${q.id}`} className="inline-block text-indigo-600 hover:underline">
        Open quotation →
      </Link>
    </div>
  );
}

// ProcurementSection adds the procurement actions visible to procurement users:
// associating quotations with the request (vendor + optional description + a
// PDF), and rejecting the request with a comment.
function ProcurementSection({ pr }: { pr: PurchaseRequest }) {
  const qc = useQueryClient();
  const { data: quotations } = useQuotationsForPR(pr.id);
  const [vendorId, setVendorId] = useState(0);
  const [description, setDescription] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [rejectComment, setRejectComment] = useState("");
  const [showReject, setShowReject] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const fileRef = useRef<HTMLInputElement>(null);

  const closed = ["rejected", "cancelled", "order_signed", "completed"].includes(pr.status);

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ["quotations", "pr", pr.id] });
    qc.invalidateQueries({ queryKey: ["quotations"] });
    qc.invalidateQueries({ queryKey: ["purchase-requests", pr.id] });
    qc.invalidateQueries({ queryKey: ["purchase-requests"] });
    qc.invalidateQueries({ queryKey: ["purchase-requests", pr.id, "related"] });
  };

  // createMutation associates a quotation with this PR and (if attached) uploads
  // its PDF, then resets the form — the user stays on the PR page.
  const createMutation = useMutation({
    mutationFn: async () => {
      if (!vendorId) throw new ApiError(400, "Please select a vendor.", null);
      const q = await createQuotation(pr.id, {
        vendor_id: vendorId,
        total_amount: 0,
        currency: "USD",
        valid_until: null,
        notes: description.trim(),
        items: [],
      });
      if (file) await uploadQuotationPDF(q.id, file);
      return q;
    },
    onSuccess: () => {
      invalidate();
      setVendorId(0);
      setDescription("");
      setFile(null);
      if (fileRef.current) fileRef.current.value = "";
      setError(null);
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : "Failed to add quotation"),
  });

  const rejectMutation = useMutation({
    mutationFn: () => rejectPurchaseRequest(pr.id, rejectComment.trim()),
    onSuccess: () => {
      invalidate();
      setShowReject(false);
      setRejectComment("");
      setError(null);
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : "Failed to reject request"),
  });

  const inputCls = "w-full rounded border px-3 py-2 text-sm focus:border-indigo-500 focus:outline-none";

  const onPickFile = (f: File | null) => {
    if (!f) {
      setFile(null);
      return;
    }
    if (!/\.pdf$/i.test(f.name)) {
      setError(`Only .pdf files are allowed (got ${f.name}).`);
      if (fileRef.current) fileRef.current.value = "";
      return;
    }
    setError(null);
    setFile(f);
  };

  return (
    <div className="app-card mt-6 p-6">
      <div className="mb-3 flex items-center justify-between">
        <h2 className="font-semibold text-slate-900">Quotations</h2>
        {!closed && !showReject && (
          <button
            onClick={() => setShowReject(true)}
            className="text-sm text-red-600 hover:underline"
          >
            Reject request
          </button>
        )}
      </div>

      {error && <p className="mb-3 text-sm text-red-600">{error}</p>}

      {showReject && (
        <div className="mb-4 space-y-2 rounded border border-red-200 bg-red-50 p-3">
          <label className="block text-sm font-medium text-gray-700">Rejection comment</label>
          <textarea
            className={inputCls}
            rows={2}
            value={rejectComment}
            onChange={(e) => setRejectComment(e.target.value)}
            placeholder="Why is this request being rejected?"
          />
          <div className="flex gap-2">
            <button
              disabled={rejectMutation.isPending || rejectComment.trim() === ""}
              onClick={() => rejectMutation.mutate()}
              className="rounded bg-red-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-red-700 disabled:opacity-50"
            >
              {rejectMutation.isPending ? "Rejecting…" : "Confirm rejection"}
            </button>
            <button
              onClick={() => {
                setShowReject(false);
                setError(null);
              }}
              className="rounded border px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-50"
            >
              Cancel
            </button>
          </div>
        </div>
      )}

      {quotations && quotations.length > 0 ? (
        <ul className="divide-y">
          {quotations.map((q) => (
            <QuotationRow key={q.id} q={q} />
          ))}
        </ul>
      ) : (
        <p className="text-sm text-gray-400">No quotations yet.</p>
      )}

      {!closed && (
        <div className="mt-6 space-y-3 rounded-lg border-2 border-indigo-300 bg-indigo-50/60 p-5 shadow-sm ring-1 ring-indigo-100">
          <div className="flex items-center gap-2">
            <span className="flex h-7 w-7 items-center justify-center rounded-full bg-indigo-600 text-sm font-bold text-white">
              +
            </span>
            <div>
              <h3 className="text-base font-semibold text-indigo-900">Add a quotation</h3>
              <p className="text-xs text-indigo-700/80">
                Record a vendor's quote against this request and attach its PDF.
              </p>
            </div>
          </div>
          <VendorSelect value={vendorId} onChange={setVendorId} />
          <textarea
            className={`${inputCls} bg-white`}
            rows={2}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="Description (optional)"
          />
          <div>
            <label className="mb-1 block text-sm font-medium text-gray-700">Quotation PDF (optional)</label>
            <input
              ref={fileRef}
              type="file"
              accept=".pdf"
              className="text-sm"
              onChange={(e) => onPickFile(e.target.files?.[0] ?? null)}
            />
          </div>
          <button
            disabled={createMutation.isPending || !vendorId}
            onClick={() => createMutation.mutate()}
            className="w-full rounded-md bg-indigo-600 px-4 py-2.5 text-sm font-semibold text-white shadow-sm transition hover:bg-indigo-700 disabled:opacity-50"
          >
            {createMutation.isPending ? "Adding…" : "Add quotation"}
          </button>
        </div>
      )}
    </div>
  );
}

function yesNo(v?: string): string {
  return v === "yes" ? "Yes" : v === "no" ? "No" : v === "unknown" ? "Don't know" : "";
}

function ReadOnlyView({ pr }: { pr: PurchaseRequest }) {
  const d = pr.details ?? {};
  const { data: budgetUnits } = useBudgetUnitLookup();
  const bu = (budgetUnits ?? []).find((c) => c.id === pr.budget_unit_id);
  const buLabel = bu
    ? bu.code
      ? `${bu.code} — ${bu.name}`
      : bu.name
    : pr.budget_unit_id
      ? `#${pr.budget_unit_id}`
      : "";
  const { data: budgetApprovers } = useBudgetUnitApprovers(
    pr.budget_unit_id,
    pr.estimated_value > 0 ? pr.estimated_value : null,
    pr.currency,
  );
  const approverText = (budgetApprovers ?? []).map((u) => u.name || u.email).join(", ");
  return (
    <div className="space-y-3 text-sm">
      <Section title="Requester" defaultOpen>
        <Row label="Reference" value={prReference(pr)} />
        <Row label="Requester" value={d.requester_name || pr.requester?.email || `#${pr.requester_id}`} />
        <Row label="Email" value={d.requester_email || pr.requester?.email} />
        <Row label="Date" value={d.date} />
        <Row label="Budget unit" value={buLabel} />
        <Row label="WSO2 entity" value={pr.entity} />
        <Row label="Business justification" value={d.business_justification} pre />
      </Section>

      <Section title="Purchase">
        <Row label="Type" value={pr.category === "IT" ? "IT solution" : pr.category === "NON-IT" ? "Non-IT solution" : ""} />
        {pr.category === "IT" && (
          <>
            <Row label="IT category" value={d.it_category} />
            <Row label="Product / solution" value={d.it_product} />
            <Row label="Description" value={d.it_description} pre />
            <Row label="Plan / tier" value={d.it_plan} />
            <Row label="Expected users" value={d.it_users} />
            <Row label="Administrators" value={d.it_admins} />
            <Row label="Day-to-day usage" value={d.it_usage} />
            <Row label="Stores sensitive data" value={yesNo(d.sec_sensitive)} />
            <Row label="Captures external PII" value={yesNo(d.sec_external_pii)} />
            {d.sec_external_pii === "yes" && <Row label="External PII detail" value={d.sec_external_pii_detail} />}
            <Row label="Captures employee PII" value={yesNo(d.sec_employee_pii)} />
            <Row label="Integrates with internal systems" value={yesNo(d.sec_integrates)} />
            {d.sec_integrates === "yes" && <Row label="Integration detail" value={d.sec_integration_detail} pre />}
          </>
        )}
        {pr.category === "NON-IT" && (
          <>
            <Row label="Non-IT category" value={d.nit_category} />
            <Row label="Details" value={d.nit_description} pre />
            <Row label="Additional specs / links" value={d.nit_specs} pre />
          </>
        )}
      </Section>

      <Section title="Vendor & budget">
        <Row label="Supplier" value={d.supplier_name} />
        <Row label="Website" value={d.supplier_website} />
        <Row label="Contact" value={d.supplier_contact} />
        <Row label="Contact email" value={d.supplier_email} />
        <Row
          label="Existing vendor"
          value={d.supplier_existing === "yes" ? "Yes — registered" : d.supplier_existing === "no" ? "No — new vendor (RFI)" : ""}
        />
        <Row label="Estimated value" value={pr.estimated_value ? formatMoney(pr.estimated_value, pr.currency) : ""} />
        <Row label="Engagement type" value={d.engagement_type} />
        <Row label="Within budget" value={yesNo(d.within_budget)} />
        <Row
          label="Budget approver"
          value={
            pr.budget_approver_name || pr.budget_approver_email
              ? `${pr.budget_approver_name || ""}${pr.budget_approver_email ? ` · ${pr.budget_approver_email}` : ""}`
              : ""
          }
        />
        <Row
          label="Designated approver"
          value={approverText}
          hint="Designated by the budget unit + estimated value; reconfirmed at the recommendation stage."
        />
        <Row label="Budget category" value={d.budget_category} />
        <Row label="Product" value={d.budget_product} />
        <Row label="Region" value={d.budget_region} />
        <Row label="Engagement code" value={d.engagement_code} />
        <Row label="Notes" value={d.notes} pre />
      </Section>

      {pr.status === "rejected" && pr.rejection_reason && (
        <div className="rounded border border-red-200 bg-red-50 px-3 py-2">
          <div className="text-xs font-semibold uppercase tracking-wide text-red-700">Rejection reason</div>
          <p className="mt-0.5 whitespace-pre-wrap text-red-700">{pr.rejection_reason}</p>
        </div>
      )}
    </div>
  );
}

// Section is an expandable group of read-only rows.
function Section({ title, defaultOpen, children }: { title: string; defaultOpen?: boolean; children: React.ReactNode }) {
  return (
    <details open={defaultOpen} className="rounded border">
      <summary className="cursor-pointer select-none px-4 py-2.5 font-medium text-gray-900">{title}</summary>
      <dl className="space-y-2 border-t px-4 py-3">{children}</dl>
    </details>
  );
}

// Row renders one label/value pair, or nothing when the value is empty.
function Row({ label, value, pre, hint }: { label: string; value?: string; pre?: boolean; hint?: string }) {
  if (!value) return null;
  return (
    <div className="sm:flex sm:gap-4">
      <dt className="w-52 shrink-0 text-gray-500">{label}</dt>
      <dd className={`text-gray-900 ${pre ? "whitespace-pre-wrap" : ""}`}>
        {value}
        {hint && <span className="mt-0.5 block text-xs italic text-gray-400">{hint}</span>}
      </dd>
    </div>
  );
}

function DocumentRow({
  prId,
  doc,
  canEdit,
  onDelete,
}: {
  prId: number;
  doc: Document;
  canEdit: boolean;
  onDelete: () => void;
}) {
  return (
    <li className="flex items-center justify-between py-2 text-sm">
      <button
        className="text-indigo-600 hover:underline"
        onClick={() => downloadDocument(prId, doc)}
      >
        {doc.filename}
      </button>
      <div className="flex items-center gap-3 text-gray-400">
        <span>{(doc.size_bytes / 1024).toFixed(0)} KB</span>
        {canEdit && (
          <button className="hover:text-red-600" onClick={onDelete}>
            Remove
          </button>
        )}
      </div>
    </li>
  );
}
