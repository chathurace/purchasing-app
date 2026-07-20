import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useQuotation } from "../hooks/useQuotations";
import { useProcurementAccess } from "../hooks/useProcurementAccess";
import { EntityStatusBadge } from "../components/EntityStatusBadge";
import { QuotationFields } from "../components/QuotationFields";
import { DocumentList } from "../components/DocumentList";
import { RelatedDocuments } from "../components/RelatedDocuments";
import { ChainStepper } from "../components/ChainStepper";
import { DirectParentCard } from "../components/CaseSections";
import {
  deleteQuotationDocument,
  deleteQuotationPDF,
  downloadQuotationDocument,
  selectQuotation,
  updateQuotation,
  uploadQuotationDocument,
  uploadQuotationPDF,
} from "../api/quotations";
import { ApiError } from "../api/client";
import { formatMoney, quoRef } from "../types/api";
import type { Document, Quotation, QuotationInput } from "../types/api";

function toInput(q: Quotation): QuotationInput {
  return {
    vendor_id: q.vendor_id,
    total_amount: q.total_amount,
    currency: q.currency,
    valid_until: q.valid_until,
    notes: q.notes,
    items: q.items.map((it) => ({
      description: it.description,
      quantity: it.quantity,
      unit_price: it.unit_price,
    })),
  };
}

export function QuotationDetailPage() {
  const { id } = useParams();
  const quoId = Number(id);
  const qc = useQueryClient();
  const procurement = useProcurementAccess();
  const { data: q, isLoading, error } = useQuotation(quoId);

  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState<QuotationInput | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ["quotations", quoId] });
    qc.invalidateQueries({ queryKey: ["quotations"] });
    if (q) qc.invalidateQueries({ queryKey: ["quotations", "pr", q.purchase_request_id] });
  };

  const saveMutation = useMutation({
    mutationFn: () => {
      const d = draft!;
      return updateQuotation(quoId, {
        ...d,
        items: d.items.filter((it) => it.description.trim() !== ""),
      });
    },
    onSuccess: () => {
      invalidate();
      setEditing(false);
      setActionError(null);
    },
    onError: (e) => setActionError(e instanceof ApiError ? e.message : "Failed to save"),
  });

  const selectMutation = useMutation({
    mutationFn: () => selectQuotation(quoId),
    onSuccess: () => {
      invalidate();
      qc.invalidateQueries({ queryKey: ["purchase-requests"] });
      setActionError(null);
    },
    onError: (e) => setActionError(e instanceof ApiError ? e.message : "Failed to select"),
  });

  const uploadMutation = useMutation({
    mutationFn: (file: File) => uploadQuotationDocument(quoId, file),
    onSuccess: invalidate,
    onError: (e) => setActionError(e instanceof ApiError ? e.message : "Upload failed"),
  });

  const deleteMutation = useMutation({
    mutationFn: (docId: number) => deleteQuotationDocument(quoId, docId),
    onSuccess: invalidate,
    onError: (e) => setActionError(e instanceof ApiError ? e.message : "Delete failed"),
  });

  const uploadPdfMutation = useMutation({
    mutationFn: (file: File) => uploadQuotationPDF(quoId, file),
    onSuccess: invalidate,
    onError: (e) => setActionError(e instanceof ApiError ? e.message : "Upload failed"),
  });

  const deletePdfMutation = useMutation({
    mutationFn: () => deleteQuotationPDF(quoId),
    onSuccess: invalidate,
    onError: (e) => setActionError(e instanceof ApiError ? e.message : "Delete failed"),
  });

  const onPickPdf = (file: File | null) => {
    if (!file) return;
    if (!/\.pdf$/i.test(file.name)) {
      setActionError(`Only .pdf files are allowed (got ${file.name}).`);
      return;
    }
    setActionError(null);
    uploadPdfMutation.mutate(file);
  };

  if (isLoading) return <p className="text-gray-500">Loading…</p>;
  if (error || !q) return <p className="text-red-600">Failed to load quotation.</p>;

  return (
    <div className="mx-auto max-w-2xl">
      <div className="mb-4 flex items-center gap-2 text-sm text-gray-500">
        <Link to="/quotations" className="text-indigo-600">
          Quotations
        </Link>
        <span>/</span>
        <span>{quoRef(q.id)}</span>
      </div>

      <ChainStepper prId={q.purchase_request_id} current={{ kind: "quotation", id: q.id }} />

      <div className="mb-4 flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold text-gray-900">
            {q.vendor?.name ?? `Vendor #${q.vendor_id}`}
          </h1>
          <div className="mt-1 flex items-center gap-2">
            <EntityStatusBadge status={q.status} />
          </div>
        </div>
        {procurement && !editing && (
          <div className="flex gap-2">
            <button
              onClick={() => {
                setDraft(toInput(q));
                setEditing(true);
              }}
              className="rounded border px-3 py-2 text-sm text-gray-700 hover:bg-gray-50"
            >
              Edit
            </button>
            {q.status !== "selected" && (
              <button
                onClick={() => selectMutation.mutate()}
                disabled={selectMutation.isPending}
                className="rounded border px-3 py-2 text-sm text-gray-700 hover:bg-gray-50 disabled:opacity-50"
              >
                Select
              </button>
            )}
          </div>
        )}
      </div>

      {actionError && <p className="mb-4 text-sm text-red-600">{actionError}</p>}

      <div className="rounded border bg-white p-6">
        {editing && draft ? (
          <>
            <QuotationFields value={draft} onChange={setDraft} />
            <div className="mt-6 flex gap-2">
              <button
                onClick={() => saveMutation.mutate()}
                disabled={saveMutation.isPending}
                className="rounded bg-indigo-600 px-4 py-2 text-sm font-medium text-white hover:bg-indigo-700 disabled:opacity-50"
              >
                {saveMutation.isPending ? "Saving…" : "Save changes"}
              </button>
              <button
                onClick={() => {
                  setEditing(false);
                  setActionError(null);
                }}
                className="rounded border px-4 py-2 text-sm text-gray-700 hover:bg-gray-50"
              >
                Cancel
              </button>
            </div>
          </>
        ) : (
          <dl className="space-y-4 text-sm">
            <Field label="Total">{formatMoney(q.total_amount, q.currency)}</Field>
            <Field label="Valid until">{q.valid_until ?? "—"}</Field>
            <Field label="Line items">
              {q.items.length === 0 ? (
                "—"
              ) : (
                <ul className="list-disc pl-5">
                  {q.items.map((it) => (
                    <li key={it.id}>
                      {it.description} —{" "}
                      <span className="text-gray-500">
                        qty {it.quantity} × {formatMoney(it.unit_price, q.currency)}
                      </span>
                    </li>
                  ))}
                </ul>
              )}
            </Field>
            <Field label="Notes">
              <span className="whitespace-pre-wrap">{q.notes || "—"}</span>
            </Field>
          </dl>
        )}
      </div>

      <DirectParentCard prId={q.purchase_request_id} current={{ kind: "quotation", id: q.id }} />

      <div className="mt-6 rounded border bg-white p-6">
        <div className="mb-3 flex items-center justify-between">
          <h2 className="font-medium text-gray-900">Quotation PDF</h2>
          {procurement && (
            <label className="cursor-pointer text-sm text-indigo-600">
              {q.quotation_document ? "Replace" : "+ Add PDF"}
              <input
                type="file"
                accept=".pdf"
                className="hidden"
                onChange={(e) => {
                  onPickPdf(e.target.files?.[0] ?? null);
                  e.target.value = "";
                }}
              />
            </label>
          )}
        </div>
        {q.quotation_document ? (
          <div className="flex items-center justify-between text-sm">
            <button
              className="text-indigo-600 hover:underline"
              onClick={() => downloadQuotationDocument(q.id, q.quotation_document!)}
            >
              📄 {q.quotation_document.filename}
            </button>
            <div className="flex items-center gap-3 text-gray-400">
              <span>{(q.quotation_document.size_bytes / 1024).toFixed(0)} KB</span>
              {procurement && (
                <button className="hover:text-red-600" onClick={() => deletePdfMutation.mutate()}>
                  Remove
                </button>
              )}
            </div>
          </div>
        ) : (
          <p className="text-sm text-gray-400">No quotation PDF attached.</p>
        )}
      </div>

      <div className="mt-6">
        <DocumentList
          documents={q.documents ?? []}
          canEdit={procurement}
          title="Other documents"
          onUpload={(f) => uploadMutation.mutate(f)}
          onDelete={(docId) => deleteMutation.mutate(docId)}
          onDownload={(doc: Document) => downloadQuotationDocument(q.id, doc)}
          onError={setActionError}
        />
      </div>

      <RelatedDocuments prId={q.purchase_request_id} current={{ kind: "quotation", id: q.id }} />
    </div>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <dt className="font-medium text-gray-500">{label}</dt>
      <dd className="mt-0.5 text-gray-900">{children}</dd>
    </div>
  );
}
