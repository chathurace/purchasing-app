import { useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useGRN } from "../hooks/useGrns";
import { GRNFields } from "../components/GRNFields";
import { DocumentList } from "../components/DocumentList";
import { ChainStepper } from "../components/ChainStepper";
import { DirectParentCard } from "../components/CaseSections";
import { RelatedDocuments } from "../components/RelatedDocuments";
import {
  deleteGRN,
  deleteGRNDocument,
  downloadGRNDocument,
  updateGRN,
  uploadGRNDocument,
} from "../api/grns";
import { ApiError } from "../api/client";
import { grnRef } from "../types/api";
import type { Document, GRN, GRNInput } from "../types/api";

function toInput(g: GRN): GRNInput {
  return {
    received_date: g.received_date,
    received_by: g.received_by,
    note: g.note,
    items: g.items.map((it) => ({ description: it.description, quantity: it.quantity })),
  };
}

export function GrnDetailPage() {
  const { id } = useParams();
  const grnId = Number(id);
  const navigate = useNavigate();
  const qc = useQueryClient();
  const { data: g, isLoading, error } = useGRN(grnId);

  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState<GRNInput | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ["grns", grnId] });
    qc.invalidateQueries({ queryKey: ["grns"] });
    if (g) qc.invalidateQueries({ queryKey: ["grns", "contract", g.contract_id] });
  };

  const saveMutation = useMutation({
    mutationFn: () => {
      const d = draft!;
      return updateGRN(grnId, { ...d, items: d.items.filter((it) => it.description.trim() !== "") });
    },
    onSuccess: () => {
      invalidate();
      setEditing(false);
      setActionError(null);
    },
    onError: (e) => setActionError(e instanceof ApiError ? e.message : "Failed to save"),
  });

  const deleteGRNMutation = useMutation({
    mutationFn: () => deleteGRN(grnId),
    onSuccess: () => {
      invalidate();
      navigate(g ? `/contracts/${g.contract_id}` : "/contracts", { replace: true });
    },
    onError: (e) => setActionError(e instanceof ApiError ? e.message : "Failed to delete GRN"),
  });

  const uploadMutation = useMutation({
    mutationFn: (file: File) => uploadGRNDocument(grnId, file),
    onSuccess: invalidate,
    onError: (e) => setActionError(e instanceof ApiError ? e.message : "Upload failed"),
  });

  const deleteDocMutation = useMutation({
    mutationFn: (docId: number) => deleteGRNDocument(grnId, docId),
    onSuccess: invalidate,
    onError: (e) => setActionError(e instanceof ApiError ? e.message : "Delete failed"),
  });

  if (isLoading) return <p className="text-gray-500">Loading…</p>;
  if (error || !g) return <p className="text-red-600">Failed to load GRN.</p>;

  return (
    <div className="mx-auto max-w-2xl">
      <div className="mb-4 flex items-center gap-2 text-sm text-gray-500">
        <Link to="/grns" className="text-indigo-600">
          GRNs
        </Link>
        <span>/</span>
        <span>{grnRef(g.id)}</span>
      </div>

      <ChainStepper prId={g.purchase_request_id} current={{ kind: "grn", id: g.id }} />

      <div className="mb-4 flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold text-gray-900">{grnRef(g.id)}</h1>
          <div className="mt-1 text-sm text-gray-500">
            {g.vendor?.name ?? `Vendor #${g.vendor_id}`} · received {g.received_date}
          </div>
        </div>
        {!editing && (
          <div className="flex gap-2">
            <button
              onClick={() => {
                setDraft(toInput(g));
                setEditing(true);
              }}
              className="rounded border px-3 py-2 text-sm text-gray-700 hover:bg-gray-50"
            >
              Edit
            </button>
            <button
              onClick={() => {
                if (confirm(`Delete ${grnRef(g.id)}? This cannot be undone.`)) deleteGRNMutation.mutate();
              }}
              disabled={deleteGRNMutation.isPending}
              className="rounded border px-3 py-2 text-sm text-red-600 hover:bg-red-50 disabled:opacity-50"
            >
              Delete
            </button>
          </div>
        )}
      </div>

      {actionError && <p className="mb-4 text-sm text-red-600">{actionError}</p>}

      <div className="rounded border bg-white p-6">
        {editing && draft ? (
          <>
            <GRNFields value={draft} onChange={setDraft} />
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
            <Field label="Received date">{g.received_date}</Field>
            <Field label="Received by">{g.received_by || "—"}</Field>
            <Field label="Items received">
              {g.items.length === 0 ? (
                "—"
              ) : (
                <ul className="list-disc pl-5">
                  {g.items.map((it) => (
                    <li key={it.id}>
                      {it.description} — <span className="text-gray-500">qty {it.quantity}</span>
                    </li>
                  ))}
                </ul>
              )}
            </Field>
            <Field label="Note">
              <span className="whitespace-pre-wrap">{g.note || "—"}</span>
            </Field>
          </dl>
        )}
      </div>

      <DirectParentCard prId={g.purchase_request_id} current={{ kind: "grn", id: g.id }} />

      <div className="mt-6">
        <DocumentList
          documents={g.documents ?? []}
          canEdit
          onUpload={(f) => uploadMutation.mutate(f)}
          onDelete={(docId) => deleteDocMutation.mutate(docId)}
          onDownload={(doc: Document) => downloadGRNDocument(g.id, doc)}
          onError={setActionError}
        />
      </div>

      <RelatedDocuments prId={g.purchase_request_id} current={{ kind: "grn", id: g.id }} />
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
