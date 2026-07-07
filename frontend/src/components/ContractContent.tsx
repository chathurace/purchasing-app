import { useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { ApiError } from "../api/client";
import {
  deleteContractDocument,
  deleteSignedDocument,
  downloadContractDocument,
  updateContractDocumentNotes,
  uploadContractDocument,
  uploadSignedDocument,
} from "../api/contracts";
import type { Contract, Document } from "../types/api";

// ContractContent renders a contract's document model: the draft-contract PDFs
// (one or more, each with its own notes) and the single signed-contract PDF
// (with notes). Shared by the contract page and the recommendation contract card.
export function ContractContent({
  contract,
  canEdit,
  invalidate,
}: {
  contract: Contract;
  canEdit: boolean;
  invalidate: () => void;
}) {
  return (
    <div className="space-y-5">
      <DraftContractsSection contract={contract} canEdit={canEdit} invalidate={invalidate} />
      <SignedContractSection contract={contract} canEdit={canEdit} invalidate={invalidate} />
    </div>
  );
}

function SectionHeading({ children }: { children: React.ReactNode }) {
  return <p className="text-xs font-semibold uppercase tracking-wide text-slate-400">{children}</p>;
}

// A single document (draft or signed): filename download, notes, and — when
// editable — inline notes editing plus a remove action.
function DocRow({
  contractId,
  doc,
  canEdit,
  removeLabel,
  onRemove,
  removing,
  invalidate,
}: {
  contractId: number;
  doc: Document;
  canEdit: boolean;
  removeLabel: string;
  onRemove: () => void;
  removing: boolean;
  invalidate: () => void;
}) {
  const [editing, setEditing] = useState(false);
  const [notes, setNotes] = useState(doc.notes);
  const [error, setError] = useState<string | null>(null);

  const saveNotes = useMutation({
    mutationFn: () => updateContractDocumentNotes(contractId, doc.id, notes.trim()),
    onSuccess: () => {
      invalidate();
      setEditing(false);
      setError(null);
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : "Failed to save notes"),
  });

  return (
    <div className="rounded-lg border border-slate-200 bg-white p-3 shadow-sm">
      <div className="flex items-start justify-between gap-3">
        <button
          type="button"
          className="inline-flex items-center gap-1.5 text-sm font-medium text-indigo-600 hover:underline"
          onClick={() => downloadContractDocument(contractId, doc)}
        >
          <span aria-hidden>📄</span>
          {doc.filename}
        </button>
        {canEdit && (
          <div className="flex shrink-0 gap-3 text-sm">
            <button
              className="font-medium text-indigo-600 hover:text-indigo-700"
              onClick={() => {
                setNotes(doc.notes);
                setError(null);
                setEditing((v) => !v);
              }}
            >
              {editing ? "Close" : doc.notes ? "Edit notes" : "Add notes"}
            </button>
            <button
              className="font-medium text-red-600 hover:text-red-700 disabled:opacity-50"
              disabled={removing}
              onClick={onRemove}
            >
              {removeLabel}
            </button>
          </div>
        )}
      </div>

      {editing ? (
        <div className="mt-2 space-y-2">
          <textarea
            className="field"
            rows={2}
            value={notes}
            onChange={(e) => setNotes(e.target.value)}
            placeholder="Notes for this document"
          />
          {error && <p className="text-sm text-red-600">{error}</p>}
          <div className="flex gap-2">
            <button
              type="button"
              disabled={saveNotes.isPending}
              onClick={() => saveNotes.mutate()}
              className="btn-primary"
            >
              {saveNotes.isPending ? "Saving…" : "Save notes"}
            </button>
            <button
              type="button"
              onClick={() => {
                setEditing(false);
                setNotes(doc.notes);
                setError(null);
              }}
              className="btn-secondary"
            >
              Cancel
            </button>
          </div>
        </div>
      ) : (
        doc.notes && <p className="mt-1.5 whitespace-pre-wrap text-sm text-slate-600">{doc.notes}</p>
      )}
    </div>
  );
}

// Upload form (a PDF + notes) shared by the draft and signed sections.
function AddDocForm({
  label,
  submitLabel,
  onSubmit,
}: {
  label: string;
  submitLabel: string;
  onSubmit: (file: File, notes: string) => Promise<unknown>;
}) {
  const [file, setFile] = useState<File | null>(null);
  const [notes, setNotes] = useState("");
  const [error, setError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: () => onSubmit(file as File, notes.trim()),
    onSuccess: () => {
      setFile(null);
      setNotes("");
      setError(null);
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : "Upload failed"),
  });

  return (
    <div className="rounded-lg border border-dashed border-slate-300 bg-slate-50/60 p-3">
      <p className="mb-2 text-sm font-medium text-slate-700">{label}</p>
      <div className="flex items-center gap-2">
        <label className="cursor-pointer text-sm font-medium text-indigo-600 hover:text-indigo-700">
          {file ? "Change PDF" : "+ Choose PDF"}
          <input
            type="file"
            accept="application/pdf,.pdf"
            className="hidden"
            onChange={(e) => setFile(e.target.files?.[0] ?? null)}
          />
        </label>
        {file && <span className="text-xs text-slate-600">{file.name}</span>}
      </div>
      <textarea
        className="field mt-2"
        rows={2}
        value={notes}
        onChange={(e) => setNotes(e.target.value)}
        placeholder="Notes (optional)"
      />
      {error && <p className="mt-2 text-sm text-red-600">{error}</p>}
      <button
        type="button"
        disabled={mutation.isPending || !file}
        onClick={() => mutation.mutate()}
        className="btn-primary mt-2"
      >
        {mutation.isPending ? "Uploading…" : submitLabel}
      </button>
    </div>
  );
}

function DraftContractsSection({
  contract,
  canEdit,
  invalidate,
}: {
  contract: Contract;
  canEdit: boolean;
  invalidate: () => void;
}) {
  const drafts = contract.documents ?? [];
  const removeMutation = useMutation({
    mutationFn: (docId: number) => deleteContractDocument(contract.id, docId),
    onSuccess: invalidate,
  });

  return (
    <div className="space-y-2">
      <SectionHeading>Draft contracts</SectionHeading>
      {drafts.length === 0 && !canEdit && <p className="text-sm text-slate-400">No draft contracts.</p>}
      {drafts.map((d) => (
        <DocRow
          key={d.id}
          contractId={contract.id}
          doc={d}
          canEdit={canEdit}
          removeLabel="Remove"
          removing={removeMutation.isPending}
          onRemove={() => removeMutation.mutate(d.id)}
          invalidate={invalidate}
        />
      ))}
      {canEdit && (
        <AddDocForm
          label="Add draft contract"
          submitLabel="Add draft contract"
          onSubmit={async (file, notes) => {
            await uploadContractDocument(contract.id, file, notes);
            invalidate();
          }}
        />
      )}
    </div>
  );
}

function SignedContractSection({
  contract,
  canEdit,
  invalidate,
}: {
  contract: Contract;
  canEdit: boolean;
  invalidate: () => void;
}) {
  const signed = contract.signed_document ?? null;
  const removeMutation = useMutation({
    mutationFn: () => deleteSignedDocument(contract.id),
    onSuccess: invalidate,
  });

  return (
    <div className="space-y-2">
      <SectionHeading>Signed contract</SectionHeading>
      {signed ? (
        <DocRow
          contractId={contract.id}
          doc={signed}
          canEdit={canEdit}
          removeLabel="Remove"
          removing={removeMutation.isPending}
          onRemove={() => removeMutation.mutate()}
          invalidate={invalidate}
        />
      ) : canEdit ? (
        <AddDocForm
          label="Attach signed contract"
          submitLabel="Attach signed contract"
          onSubmit={async (file, notes) => {
            await uploadSignedDocument(contract.id, file, notes);
            invalidate();
          }}
        />
      ) : (
        <p className="text-sm text-slate-400">Not signed yet.</p>
      )}
    </div>
  );
}
