import { useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useInvoice } from "../hooks/useInvoices";
import { EntityStatusBadge } from "../components/EntityStatusBadge";
import { InvoiceFields } from "../components/InvoiceFields";
import { DocumentList } from "../components/DocumentList";
import { ChainStepper } from "../components/ChainStepper";
import { DirectParentCard } from "../components/CaseSections";
import { RelatedDocuments } from "../components/RelatedDocuments";
import {
  deleteInvoice,
  deleteInvoiceDocument,
  downloadInvoiceDocument,
  setInvoiceStatus,
  updateInvoice,
  uploadInvoiceDocument,
} from "../api/invoices";
import { ApiError } from "../api/client";
import { allocationsValid, formatMoney, invRef, invoiceEffectiveTotal, invoiceItemsTotal } from "../types/api";
import type { Document, Invoice, InvoiceInput, InvoiceStatus } from "../types/api";

function toInput(inv: Invoice): InvoiceInput {
  return {
    vendor_invoice_no: inv.vendor_invoice_no,
    invoice_date: inv.invoice_date,
    due_date: inv.due_date,
    currency: inv.currency,
    note: inv.note,
    allocation_mode: inv.allocation_mode,
    entered_total: inv.entered_total,
    items: inv.items.map((it) => ({
      description: it.description,
      quantity: it.quantity,
      unit_price: it.unit_price,
    })),
    cost_allocations: inv.cost_allocations.map((a) => ({
      budget_unit_id: a.budget_unit_id,
      value: a.value,
    })),
  };
}

export function InvoiceDetailPage() {
  const { id } = useParams();
  const invId = Number(id);
  const navigate = useNavigate();
  const qc = useQueryClient();
  const { data: inv, isLoading, error } = useInvoice(invId);

  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState<InvoiceInput | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ["invoices", invId] });
    qc.invalidateQueries({ queryKey: ["invoices"] });
    if (inv) {
      qc.invalidateQueries({ queryKey: ["invoices", "contract", inv.contract_id] });
      qc.invalidateQueries({ queryKey: ["contracts", inv.contract_id] });
    }
  };

  const saveMutation = useMutation({
    mutationFn: () => {
      const d = draft!;
      return updateInvoice(invId, { ...d, items: d.items.filter((it) => it.description.trim() !== "") });
    },
    onSuccess: () => {
      invalidate();
      setEditing(false);
      setActionError(null);
    },
    onError: (e) => setActionError(e instanceof ApiError ? e.message : "Failed to save"),
  });

  const statusMutation = useMutation({
    mutationFn: (status: InvoiceStatus) => setInvoiceStatus(invId, status),
    onSuccess: () => {
      invalidate();
      setActionError(null);
    },
    onError: (e) => setActionError(e instanceof ApiError ? e.message : "Failed to update status"),
  });

  const deleteInvoiceMutation = useMutation({
    mutationFn: () => deleteInvoice(invId),
    onSuccess: () => {
      invalidate();
      navigate(inv ? `/contracts/${inv.contract_id}` : "/invoices", { replace: true });
    },
    onError: (e) => setActionError(e instanceof ApiError ? e.message : "Failed to delete invoice"),
  });

  const uploadMutation = useMutation({
    mutationFn: (file: File) => uploadInvoiceDocument(invId, file),
    onSuccess: invalidate,
    onError: (e) => setActionError(e instanceof ApiError ? e.message : "Upload failed"),
  });

  const deleteDocMutation = useMutation({
    mutationFn: (docId: number) => deleteInvoiceDocument(invId, docId),
    onSuccess: invalidate,
    onError: (e) => setActionError(e instanceof ApiError ? e.message : "Delete failed"),
  });

  if (isLoading) return <p className="text-gray-500">Loading…</p>;
  if (error || !inv) return <p className="text-red-600">Failed to load invoice.</p>;

  const isReceived = inv.status === "received";
  const setStatus = (s: InvoiceStatus) => statusMutation.mutate(s);

  return (
    <div className="mx-auto max-w-2xl">
      <div className="mb-4 flex items-center gap-2 text-sm text-gray-500">
        <Link to="/invoices" className="text-indigo-600">
          Invoices
        </Link>
        <span>/</span>
        <span>{invRef(inv.id)}</span>
      </div>

      <ChainStepper prId={inv.purchase_request_id} current={{ kind: "invoice", id: inv.id }} />

      <div className="mb-4 flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold text-gray-900">
            {inv.vendor_invoice_no || invRef(inv.id)}
          </h1>
          <div className="mt-1 flex items-center gap-2">
            <EntityStatusBadge status={inv.status} />
            <span className="text-sm text-gray-500">
              {inv.vendor?.name ?? `Vendor #${inv.vendor_id}`} · {formatMoney(inv.total_amount, inv.currency)}
            </span>
          </div>
        </div>
        {!editing && (
          <div className="flex flex-wrap justify-end gap-2">
            {isReceived && (
              <button
                onClick={() => {
                  setDraft(toInput(inv));
                  setEditing(true);
                }}
                className="rounded border px-3 py-2 text-sm text-gray-700 hover:bg-gray-50"
              >
                Edit
              </button>
            )}
            {/* Status transitions */}
            {inv.status === "received" && (
              <button
                onClick={() => setStatus("approved")}
                disabled={statusMutation.isPending}
                className="rounded bg-indigo-600 px-3 py-2 text-sm font-medium text-white hover:bg-indigo-700 disabled:opacity-50"
              >
                Approve
              </button>
            )}
            {inv.status === "approved" && (
              <>
                <button
                  onClick={() => setStatus("received")}
                  disabled={statusMutation.isPending}
                  className="rounded border px-3 py-2 text-sm text-gray-700 hover:bg-gray-50 disabled:opacity-50"
                >
                  Revert to received
                </button>
                <button
                  onClick={() => setStatus("paid")}
                  disabled={statusMutation.isPending}
                  className="rounded bg-green-600 px-3 py-2 text-sm font-medium text-white hover:bg-green-700 disabled:opacity-50"
                >
                  Mark as paid
                </button>
              </>
            )}
            {inv.status === "paid" && (
              <button
                onClick={() => setStatus("approved")}
                disabled={statusMutation.isPending}
                className="rounded border px-3 py-2 text-sm text-gray-700 hover:bg-gray-50 disabled:opacity-50"
              >
                Revert to approved
              </button>
            )}
            {isReceived && (
              <button
                onClick={() => {
                  if (confirm(`Delete ${invRef(inv.id)}? This cannot be undone.`)) deleteInvoiceMutation.mutate();
                }}
                disabled={deleteInvoiceMutation.isPending}
                className="rounded border px-3 py-2 text-sm text-red-600 hover:bg-red-50 disabled:opacity-50"
              >
                Delete
              </button>
            )}
          </div>
        )}
      </div>

      {actionError && <p className="mb-4 text-sm text-red-600">{actionError}</p>}

      <div className="rounded border bg-white p-6">
        {editing && draft ? (
          <>
            <InvoiceFields value={draft} onChange={setDraft} />
            <div className="mt-6 flex gap-2">
              <button
                onClick={() => saveMutation.mutate()}
                disabled={
                  saveMutation.isPending ||
                  !allocationsValid(draft.allocation_mode, draft.cost_allocations, invoiceEffectiveTotal(draft))
                }
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
            <Field label="Invoice date">{inv.invoice_date}</Field>
            <Field label="Due date">{inv.due_date ?? "—"}</Field>
            <Field label="Total">
              {formatMoney(inv.total_amount, inv.currency)}
              {inv.entered_total != null && inv.items.length > 0 &&
                Math.abs(inv.entered_total - invoiceItemsTotal(inv.items)) > 0.01 && (
                  <span className="ml-2 text-xs text-gray-500">
                    (entered; line items total {formatMoney(invoiceItemsTotal(inv.items), inv.currency)})
                  </span>
                )}
            </Field>
            <Field label="Line items">
              {inv.items.length === 0 ? (
                "—"
              ) : (
                <ul className="list-disc pl-5">
                  {inv.items.map((it) => (
                    <li key={it.id}>
                      {it.description} —{" "}
                      <span className="text-gray-500">
                        qty {it.quantity} × {formatMoney(it.unit_price, inv.currency)}
                      </span>
                    </li>
                  ))}
                </ul>
              )}
            </Field>
            <Field label={`Budget units (by ${inv.allocation_mode})`}>
              {inv.cost_allocations.length === 0 ? (
                "—"
              ) : (
                <ul className="list-disc pl-5">
                  {inv.cost_allocations.map((a) => (
                    <li key={a.id ?? a.budget_unit_id}>
                      {a.budget_unit
                        ? a.budget_unit.code
                          ? `${a.budget_unit.code} — ${a.budget_unit.name}`
                          : a.budget_unit.name
                        : `#${a.budget_unit_id}`}{" "}
                      <span className="text-gray-500">
                        {inv.allocation_mode === "percentage"
                          ? `${a.value}% (${formatMoney(a.amount ?? 0, inv.currency)})`
                          : formatMoney(a.amount ?? a.value, inv.currency)}
                      </span>
                    </li>
                  ))}
                </ul>
              )}
            </Field>
            {inv.approver && (
              <Field label="Approved by">
                {inv.approver.name || inv.approver.email}
              </Field>
            )}
            {inv.paid_date && <Field label="Paid on">{inv.paid_date}</Field>}
            <Field label="Note">
              <span className="whitespace-pre-wrap">{inv.note || "—"}</span>
            </Field>
          </dl>
        )}
      </div>

      <DirectParentCard prId={inv.purchase_request_id} current={{ kind: "invoice", id: inv.id }} />

      <div className="mt-6">
        <DocumentList
          documents={inv.documents ?? []}
          canEdit
          title="Invoice documents"
          onUpload={(f) => uploadMutation.mutate(f)}
          onDelete={(docId) => deleteDocMutation.mutate(docId)}
          onDownload={(doc: Document) => downloadInvoiceDocument(inv.id, doc)}
          onError={setActionError}
        />
      </div>

      <RelatedDocuments prId={inv.purchase_request_id} current={{ kind: "invoice", id: inv.id }} />
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
