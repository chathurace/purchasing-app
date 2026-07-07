import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useFinanceAccess } from "../hooks/useFinanceAccess";
import { useQuotationsForPR } from "../hooks/useQuotations";
import { ApiError } from "../api/client";
import {
  addRecComment,
  createRecommendation,
  createRecommendationContract,
  deleteRecommendation,
  deleteRecommendationContract,
  deleteRecommendationRFI,
  deleteRecRFIDocument,
  downloadRecCommentDocument,
  downloadRecRFIDocument,
  setRecApproval,
  setRecommendationRFI,
  updateRecommendation,
  uploadRecCommentDocument,
  uploadRecRFIDocument,
} from "../api/recommendations";
import { uploadContractDocument } from "../api/contracts";
import { ContractContent } from "./ContractContent";
import { EntityStatusBadge } from "./EntityStatusBadge";
import { conRef, REC_APPROVAL_LABELS, REC_APPROVAL_TYPES } from "../types/api";
import type {
  Document,
  PurchaseRequest,
  RecApproval,
  RecApprovalType,
  Recommendation,
} from "../types/api";

const inputCls = "w-full rounded border px-3 py-2 text-sm focus:border-indigo-500 focus:outline-none";

interface QuotedVendor {
  id: number;
  name: string;
  registered: boolean;
}

// RecommendationSection renders the PR's procurement recommendation: finance adds
// or edits it (once at least one quotation exists), and the named actors
// (legal/security/budget owner) toggle approval and comment on their card.
export function RecommendationSection({ pr }: { pr: PurchaseRequest }) {
  const finance = useFinanceAccess();
  // Quoted vendors feed the create/edit form (finance only); a non-finance actor
  // acting on a card never needs them, so skip the finance-gated fetch for them.
  const { data: quotations } = useQuotationsForPR(pr.id, finance);
  const rec = pr.recommendation ?? null;

  const quotedVendors: QuotedVendor[] = useMemo(() => {
    const seen = new Map<number, { name: string; registered: boolean }>();
    for (const q of quotations ?? []) {
      if (!seen.has(q.vendor_id))
        seen.set(q.vendor_id, {
          name: q.vendor?.name ?? `Vendor #${q.vendor_id}`,
          registered: q.vendor?.registered ?? false,
        });
    }
    return Array.from(seen, ([id, { name, registered }]) => ({ id, name, registered }));
  }, [quotations]);

  // Nothing to show for a non-finance viewer until a recommendation exists.
  if (!rec && !finance) return null;
  // Finance can only add a recommendation once a quotation is in.
  if (!rec && quotedVendors.length === 0) return null;

  return (
    <div className="app-card mt-6 p-6">
      <h2 className="mb-3 font-semibold text-slate-900">Procurement recommendation</h2>
      {rec ? (
        <RecommendationView pr={pr} rec={rec} finance={finance} quotedVendors={quotedVendors} />
      ) : (
        <RecommendationForm pr={pr} quotedVendors={quotedVendors} />
      )}
    </div>
  );
}

// --- create / edit form ---

function RecommendationForm({
  pr,
  quotedVendors,
  existing,
  onDone,
}: {
  pr: PurchaseRequest;
  quotedVendors: QuotedVendor[];
  existing?: Recommendation;
  onDone?: () => void;
}) {
  const qc = useQueryClient();
  const editing = !!existing;
  const [vendorId, setVendorId] = useState(existing?.vendor_id ?? 0);
  const [description, setDescription] = useState(existing?.description ?? "");
  const [types, setTypes] = useState<RecApprovalType[]>(
    existing ? existing.approvals.map((a) => a.approval_type) : ["budget"],
  );
  const [error, setError] = useState<string | null>(null);

  const invalidate = () => qc.invalidateQueries({ queryKey: ["purchase-requests", pr.id] });

  const mutation = useMutation({
    mutationFn: () => {
      const input = { vendor_id: vendorId, description: description.trim(), required_types: types };
      return editing ? updateRecommendation(pr.id, input) : createRecommendation(pr.id, input);
    },
    onSuccess: () => {
      invalidate();
      setError(null);
      onDone?.();
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : "Failed to save recommendation"),
  });

  const toggleType = (t: RecApprovalType) =>
    setTypes((prev) => (prev.includes(t) ? prev.filter((x) => x !== t) : [...prev, t]));

  return (
    <div className="space-y-3">
      {editing && (
        <p className="rounded border-l-2 border-amber-300 bg-amber-50 px-3 py-2 text-xs text-amber-800">
          Editing the recommendation resets all approvals back to pending.
        </p>
      )}
      <div>
        <label className="mb-1 block text-sm font-medium text-gray-700">Vendor</label>
        <select className={inputCls} value={vendorId || ""} onChange={(e) => setVendorId(Number(e.target.value))}>
          <option value="">Select a quoted vendor…</option>
          {quotedVendors.map((v) => (
            <option key={v.id} value={v.id}>
              {v.name}
              {!v.registered ? " (unregistered)" : ""}
            </option>
          ))}
        </select>
      </div>
      <div>
        <label className="mb-1 block text-sm font-medium text-gray-700">Description</label>
        <textarea
          className={inputCls}
          rows={2}
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder="Why this vendor (optional)"
        />
      </div>
      <div>
        <span className="mb-1 block text-sm font-medium text-gray-700">Approvals required</span>
        <div className="flex flex-wrap gap-4">
          {REC_APPROVAL_TYPES.map((t) => (
            <label key={t} className="flex items-center gap-2 text-sm text-gray-700">
              <input type="checkbox" checked={types.includes(t)} onChange={() => toggleType(t)} />
              {REC_APPROVAL_LABELS[t]}
            </label>
          ))}
        </div>
      </div>
      {error && <p className="text-sm text-red-600">{error}</p>}
      <div className="flex gap-2 pt-1">
        <button
          disabled={mutation.isPending || !vendorId || types.length === 0}
          onClick={() => mutation.mutate()}
          className="rounded bg-indigo-600 px-4 py-2 text-sm font-medium text-white hover:bg-indigo-700 disabled:opacity-50"
        >
          {mutation.isPending ? "Saving…" : editing ? "Save changes" : "Save recommendation"}
        </button>
        {editing && (
          <button
            onClick={() => onDone?.()}
            className="rounded border px-4 py-2 text-sm text-gray-700 hover:bg-gray-50"
          >
            Cancel
          </button>
        )}
      </div>
    </div>
  );
}

// --- read view with approval cards ---

function RecommendationView({
  pr,
  rec,
  finance,
  quotedVendors,
}: {
  pr: PurchaseRequest;
  rec: Recommendation;
  finance: boolean;
  quotedVendors: QuotedVendor[];
}) {
  const qc = useQueryClient();
  const [editing, setEditing] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const invalidate = () => qc.invalidateQueries({ queryKey: ["purchase-requests", pr.id] });

  const deleteMutation = useMutation({
    mutationFn: () => deleteRecommendation(pr.id),
    onSuccess: invalidate,
    onError: (e) => setError(e instanceof ApiError ? e.message : "Failed to remove recommendation"),
  });

  if (editing) {
    return (
      <RecommendationForm
        pr={pr}
        quotedVendors={quotedVendors}
        existing={rec}
        onDone={() => setEditing(false)}
      />
    );
  }

  const approved = rec.approvals.filter((a) => a.approved).length;

  return (
    <div className="space-y-4">
      <div className="flex items-start justify-between gap-3">
        <dl className="space-y-1 text-sm">
          <div>
            <dt className="inline font-medium text-slate-500">Vendor: </dt>
            <dd className="inline font-medium text-slate-900">{rec.vendor?.name ?? `Vendor #${rec.vendor_id}`}</dd>
          </div>
          {rec.description && (
            <p className="whitespace-pre-wrap text-slate-600">{rec.description}</p>
          )}
          <p className="pt-0.5">
            <span
              className={`badge ${
                approved === rec.approvals.length
                  ? "bg-emerald-50 text-emerald-700 ring-emerald-600/20"
                  : "bg-slate-100 text-slate-600 ring-slate-500/20"
              }`}
            >
              {approved} of {rec.approvals.length} approvals granted
            </span>
          </p>
        </dl>
        {finance && (
          <div className="flex shrink-0 gap-3 text-sm">
            <button className="font-medium text-indigo-600 hover:text-indigo-700" onClick={() => setEditing(true)}>
              Edit
            </button>
            <button
              className="font-medium text-red-600 hover:text-red-700 disabled:opacity-50"
              disabled={deleteMutation.isPending}
              onClick={() => deleteMutation.mutate()}
            >
              Remove
            </button>
          </div>
        )}
      </div>

      {error && <p className="text-sm text-red-600">{error}</p>}

      <div>
        <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-slate-400">
          Required approvals
        </p>
        <div className="space-y-3">
          {rec.approvals.map((a) => (
            <ApprovalCard
              key={a.approval_type}
              prId={pr.id}
              approval={a}
              canAct={rec.my_actionable_types.includes(a.approval_type)}
            />
          ))}
        </div>
      </div>

      <RFICard pr={pr} rec={rec} finance={finance} />
      <ContractCard pr={pr} rec={rec} finance={finance} />
    </div>
  );
}

// --- shared chrome for the recommendation's sub-cards ---
//
// Each sub-card (approval / RFI / contract) is an elevated white card with a
// colored left accent and an icon tile, so the individual steps of the
// procurement function read as distinct, primary components.

const TONE: Record<
  "emerald" | "amber" | "indigo" | "violet" | "sky" | "slate",
  { bar: string; tile: string }
> = {
  emerald: { bar: "border-l-emerald-400", tile: "bg-emerald-50 text-emerald-600 ring-emerald-600/20" },
  amber: { bar: "border-l-amber-400", tile: "bg-amber-50 text-amber-600 ring-amber-600/20" },
  indigo: { bar: "border-l-indigo-400", tile: "bg-indigo-50 text-indigo-600 ring-indigo-600/20" },
  violet: { bar: "border-l-violet-400", tile: "bg-violet-50 text-violet-600 ring-violet-600/20" },
  sky: { bar: "border-l-sky-400", tile: "bg-sky-50 text-sky-600 ring-sky-600/20" },
  slate: { bar: "border-l-slate-300", tile: "bg-slate-100 text-slate-500 ring-slate-500/20" },
};

type Tone = keyof typeof TONE;

// SubCard is the elevated shell shared by the recommendation's sub-cards.
function SubCard({ tone, children }: { tone: Tone; children: React.ReactNode }) {
  return (
    <div
      className={`overflow-hidden rounded-xl border border-l-4 border-slate-200 bg-white shadow-sm transition hover:shadow-md ${TONE[tone].bar}`}
    >
      {children}
    </div>
  );
}

function IconTile({ tone, children }: { tone: Tone; children: React.ReactNode }) {
  return (
    <span
      className={`flex h-9 w-9 shrink-0 items-center justify-center rounded-lg ring-1 ring-inset ${TONE[tone].tile}`}
    >
      {children}
    </span>
  );
}

// SubCardBody is the tinted lower section (comments, form fields, attachments).
function SubCardBody({ children }: { children: React.ReactNode }) {
  return <div className="border-t border-slate-100 bg-slate-50/60 px-4 py-3">{children}</div>;
}

const svgCls = "h-[18px] w-[18px]";
const svgProps = {
  className: svgCls,
  viewBox: "0 0 24 24",
  fill: "none",
  stroke: "currentColor",
  strokeWidth: 1.8,
  strokeLinecap: "round" as const,
  strokeLinejoin: "round" as const,
};

function ApprovalIcon({ type }: { type: string }) {
  if (type === "legal") {
    // scales of justice
    return (
      <svg {...svgProps}>
        <path d="M12 3v18M7 21h10M5 7h14M12 3 5 7l-2.5 5a3 3 0 0 0 6 0L12 3zm0 0 7 4 2.5 5a3 3 0 0 1-6 0L12 3z" />
      </svg>
    );
  }
  if (type === "security") {
    // shield with check
    return (
      <svg {...svgProps}>
        <path d="M12 3 5 6v5c0 4.5 3 8 7 10 4-2 7-5.5 7-10V6l-7-3z" />
        <path d="m9 12 2 2 4-4" />
      </svg>
    );
  }
  // budget — banknote
  return (
    <svg {...svgProps}>
      <rect x="2" y="6" width="20" height="12" rx="2" />
      <circle cx="12" cy="12" r="2.5" />
      <path d="M6 12h.01M18 12h.01" />
    </svg>
  );
}

function RFIIcon() {
  // chat bubble with a question
  return (
    <svg {...svgProps}>
      <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />
      <path d="M9.5 9a2.5 2.5 0 1 1 3 2.5c-.7.3-1 .8-1 1.5M12 16h.01" />
    </svg>
  );
}

function ContractDocIcon() {
  return (
    <svg {...svgProps}>
      <path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z" />
      <path d="M14 3v5h5M9 13h6M9 17h4" />
    </svg>
  );
}

// --- optional RFI card: request for information from the vendor (description + PDFs) ---

function DocChip({ filename, onClick }: { filename: string; onClick: () => void }) {
  return (
    <button
      type="button"
      className="inline-flex items-center gap-1 rounded-full border bg-white px-2 py-0.5 text-xs text-indigo-600 hover:bg-gray-100"
      onClick={onClick}
    >
      <span aria-hidden>📎</span>
      {filename}
    </button>
  );
}

function RFICard({
  pr,
  rec,
  finance,
}: {
  pr: PurchaseRequest;
  rec: Recommendation;
  finance: boolean;
}) {
  const qc = useQueryClient();
  const present = rec.rfi_description.trim() !== "" || (rec.rfi_documents?.length ?? 0) > 0;
  const [editing, setEditing] = useState(false);
  const [description, setDescription] = useState(rec.rfi_description);
  const [file, setFile] = useState<File | null>(null);
  const [error, setError] = useState<string | null>(null);

  const invalidate = () => qc.invalidateQueries({ queryKey: ["purchase-requests", pr.id] });

  // Save = set the description, then (if chosen) upload a new attachment.
  const saveMutation = useMutation({
    mutationFn: async () => {
      await setRecommendationRFI(pr.id, description.trim());
      if (file) await uploadRecRFIDocument(pr.id, file);
    },
    onSuccess: () => {
      invalidate();
      setFile(null);
      setEditing(false);
      setError(null);
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : "Failed to save RFI"),
  });

  const removeMutation = useMutation({
    mutationFn: () => deleteRecommendationRFI(pr.id),
    onSuccess: () => {
      invalidate();
      setDescription("");
      setError(null);
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : "Failed to remove RFI"),
  });

  const removeDocMutation = useMutation({
    mutationFn: (docId: number) => deleteRecRFIDocument(pr.id, docId),
    onSuccess: invalidate,
    onError: (e) => setError(e instanceof ApiError ? e.message : "Failed to remove attachment"),
  });

  // Add-attachment shortcut from the read view (no description change).
  const addDocMutation = useMutation({
    mutationFn: (f: File) => uploadRecRFIDocument(pr.id, f),
    onSuccess: invalidate,
    onError: (e) => setError(e instanceof ApiError ? e.message : "Failed to add attachment"),
  });

  // Nothing to show for a non-finance viewer when there's no RFI.
  if (!present && !finance) return null;

  // Finance editing (or first-time add) form.
  if (finance && (editing || !present)) {
    return (
      <SubCard tone="sky">
        <div className="flex items-center gap-3 px-4 py-3">
          <IconTile tone="sky">
            <RFIIcon />
          </IconTile>
          <div className="min-w-0">
            <p className="text-sm font-semibold text-slate-900">RFI — request for information</p>
            <p className="text-xs text-slate-500">
              {present ? "Update the request or its attachments" : "Optional — ask the vendor for more detail"}
            </p>
          </div>
        </div>
        <SubCardBody>
          <textarea
            className={inputCls}
            rows={2}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="What information are you requesting from the vendor?"
          />
          <div className="mt-2 flex items-center gap-2">
            <label className="cursor-pointer text-sm font-medium text-indigo-600 hover:text-indigo-700">
              {file ? "Change PDF" : "+ Attach PDF"}
              <input
                type="file"
                accept="application/pdf,.pdf"
                className="hidden"
                onChange={(e) => setFile(e.target.files?.[0] ?? null)}
              />
            </label>
            {file && <span className="text-xs text-slate-600">{file.name}</span>}
          </div>
          {error && <p className="mt-2 text-sm text-red-600">{error}</p>}
          <div className="mt-3 flex gap-2">
            <button
              type="button"
              disabled={saveMutation.isPending || (!present && !file && description.trim() === "")}
              onClick={() => saveMutation.mutate()}
              className="btn-primary"
            >
              {saveMutation.isPending ? "Saving…" : present ? "Save RFI" : "Add RFI"}
            </button>
            {present && (
              <button
                type="button"
                onClick={() => {
                  setEditing(false);
                  setDescription(rec.rfi_description);
                  setFile(null);
                  setError(null);
                }}
                className="btn-secondary"
              >
                Cancel
              </button>
            )}
          </div>
        </SubCardBody>
      </SubCard>
    );
  }

  // Read view (RFI present).
  return (
    <SubCard tone="sky">
      <div className="flex items-center gap-3 px-4 py-3">
        <IconTile tone="sky">
          <RFIIcon />
        </IconTile>
        <div className="min-w-0">
          <p className="text-sm font-semibold text-slate-900">RFI — request for information</p>
          <p className="text-xs text-slate-500">Raised with the vendor</p>
        </div>
        {finance && (
          <div className="ml-auto flex shrink-0 gap-3 text-sm">
            <button className="font-medium text-indigo-600 hover:text-indigo-700" onClick={() => setEditing(true)}>
              Edit
            </button>
            <button
              className="font-medium text-red-600 hover:text-red-700 disabled:opacity-50"
              disabled={removeMutation.isPending}
              onClick={() => removeMutation.mutate()}
            >
              Remove
            </button>
          </div>
        )}
      </div>
      <SubCardBody>
        {rec.rfi_description && (
          <p className="whitespace-pre-wrap text-sm text-slate-700">{rec.rfi_description}</p>
        )}
        {(rec.rfi_documents?.length ?? 0) > 0 && (
          <div className={`flex flex-wrap items-center gap-1.5 ${rec.rfi_description ? "mt-2" : ""}`}>
            {rec.rfi_documents.map((d) => (
              <span key={d.id} className="inline-flex items-center">
                <DocChip filename={d.filename} onClick={() => downloadRecRFIDocument(pr.id, d)} />
                {finance && (
                  <button
                    type="button"
                    title="Remove attachment"
                    disabled={removeDocMutation.isPending}
                    onClick={() => removeDocMutation.mutate(d.id)}
                    className="ml-0.5 text-xs text-slate-400 hover:text-red-600 disabled:opacity-50"
                  >
                    ✕
                  </button>
                )}
              </span>
            ))}
          </div>
        )}
        {finance && (
          <label className="mt-2 inline-block cursor-pointer text-sm font-medium text-indigo-600 hover:text-indigo-700">
            + Add attachment
            <input
              type="file"
              accept="application/pdf,.pdf"
              className="hidden"
              onChange={(e) => {
                const f = e.target.files?.[0];
                if (f) addDocMutation.mutate(f);
                e.target.value = "";
              }}
            />
          </label>
        )}
        {error && <p className="mt-2 text-sm text-red-600">{error}</p>}
      </SubCardBody>
    </SubCard>
  );
}

// --- optional contract card: attach a contract PDF + description ---

function ContractCard({
  pr,
  rec,
  finance,
}: {
  pr: PurchaseRequest;
  rec: Recommendation;
  finance: boolean;
}) {
  const qc = useQueryClient();
  const contract = rec.contract ?? null;

  // First-draft form state (no contract yet).
  const [file, setFile] = useState<File | null>(null);
  const [notes, setNotes] = useState("");
  const [error, setError] = useState<string | null>(null);

  const invalidate = () => qc.invalidateQueries({ queryKey: ["purchase-requests", pr.id] });

  // Adding the first draft creates the contract row, then uploads the draft PDF.
  const createMutation = useMutation({
    mutationFn: async () => {
      const c = await createRecommendationContract(pr.id, "");
      if (file) await uploadContractDocument(c.id, file, notes.trim());
    },
    onSuccess: () => {
      invalidate();
      setFile(null);
      setNotes("");
      setError(null);
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : "Failed to add contract"),
  });

  const removeMutation = useMutation({
    mutationFn: () => deleteRecommendationContract(pr.id),
    onSuccess: () => {
      invalidate();
      setError(null);
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : "Failed to remove contract"),
  });

  if (contract) {
    // The whole contract can be removed only while it has no signed PDF (draft).
    const canRemove = finance && contract.status === "draft";
    return (
      <SubCard tone="violet">
        <div className="flex items-center gap-3 px-4 py-3">
          <IconTile tone="violet">
            <ContractDocIcon />
          </IconTile>
          <div className="min-w-0">
            <Link
              to={`/contracts/${contract.id}`}
              className="text-sm font-semibold text-indigo-600 hover:underline"
            >
              {conRef(contract.id)}
            </Link>
            <p className="text-xs text-slate-500">Contract</p>
          </div>
          <div className="ml-auto flex shrink-0 items-center gap-3">
            <EntityStatusBadge status={contract.status} />
            {canRemove && (
              <button
                className="text-sm font-medium text-red-600 hover:text-red-700 disabled:opacity-50"
                disabled={removeMutation.isPending}
                onClick={() => removeMutation.mutate()}
              >
                Remove
              </button>
            )}
          </div>
        </div>
        <SubCardBody>
          {error && <p className="mb-2 text-sm text-red-600">{error}</p>}
          <ContractContent contract={contract} canEdit={finance} invalidate={invalidate} />
        </SubCardBody>
      </SubCard>
    );
  }

  if (!finance) return null;

  return (
    <SubCard tone="violet">
      <div className="flex items-center gap-3 px-4 py-3">
        <IconTile tone="violet">
          <ContractDocIcon />
        </IconTile>
        <div className="min-w-0">
          <p className="text-sm font-semibold text-slate-900">Contract</p>
          <p className="text-xs text-slate-500">Optional — attach a draft contract PDF to begin</p>
        </div>
      </div>
      <SubCardBody>
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
          className={`${inputCls} mt-2`}
          rows={2}
          value={notes}
          onChange={(e) => setNotes(e.target.value)}
          placeholder="Notes for this draft (optional)"
        />
        {error && <p className="mt-2 text-sm text-red-600">{error}</p>}
        <button
          type="button"
          disabled={createMutation.isPending || !file}
          onClick={() => createMutation.mutate()}
          className="btn-primary mt-3"
        >
          {createMutation.isPending ? "Adding…" : "Add draft contract"}
        </button>
      </SubCardBody>
    </SubCard>
  );
}

// --- one approval card: toggle + comment thread ---

function reviewerLabel(name?: string | null, email?: string | null, id?: number): string {
  return email || name || (id != null ? `#${id}` : "");
}

function ApprovalCard({
  prId,
  approval,
  canAct,
}: {
  prId: number;
  approval: RecApproval;
  canAct: boolean;
}) {
  const qc = useQueryClient();
  const t = approval.approval_type;
  const [comment, setComment] = useState("");
  const [files, setFiles] = useState<File[]>([]);
  const [open, setOpen] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const invalidate = () => qc.invalidateQueries({ queryKey: ["purchase-requests", prId] });

  const toggleMutation = useMutation({
    mutationFn: () => setRecApproval(prId, t, !approval.approved),
    onSuccess: invalidate,
    onError: (e) => setError(e instanceof ApiError ? e.message : "Failed to update approval"),
  });

  const commentMutation = useMutation({
    mutationFn: async () => {
      const c = await addRecComment(prId, t, comment.trim());
      for (const f of files) await uploadRecCommentDocument(prId, c.id, f);
    },
    onSuccess: () => {
      invalidate();
      setComment("");
      setFiles([]);
      setOpen(false);
      setError(null);
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : "Failed to add comment"),
  });

  const tone: Tone = approval.approved ? "emerald" : "amber";
  return (
    <SubCard tone={tone}>
      <div className="flex items-center gap-3 px-4 py-3">
        <IconTile tone={tone}>
          <ApprovalIcon type={t} />
        </IconTile>
        <div className="min-w-0">
          <p className="text-sm font-semibold text-slate-900">{REC_APPROVAL_LABELS[t]}</p>
          <p className="truncate text-xs text-slate-500">
            {approval.approved && approval.approver
              ? `Approved by ${reviewerLabel(approval.approver.name, approval.approver.email)}`
              : "Awaiting sign-off"}
          </p>
        </div>
        <div className="ml-auto flex shrink-0 items-center gap-3">
          <span
            className={`badge ${
              approval.approved
                ? "bg-emerald-50 text-emerald-700 ring-emerald-600/20"
                : "bg-amber-50 text-amber-700 ring-amber-600/20"
            }`}
          >
            <span className={`h-1.5 w-1.5 rounded-full ${approval.approved ? "bg-emerald-500" : "bg-amber-500"}`} />
            {approval.approved ? "Approved" : "Pending"}
          </span>
          {canAct && (
            <button
              type="button"
              role="switch"
              aria-checked={approval.approved}
              disabled={toggleMutation.isPending}
              onClick={() => toggleMutation.mutate()}
              title={approval.approved ? "Revert to pending" : "Approve"}
              className={`relative inline-flex h-5 w-9 items-center rounded-full transition-colors disabled:opacity-50 ${
                approval.approved ? "bg-emerald-600" : "bg-slate-300"
              }`}
            >
              <span
                className={`inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform ${
                  approval.approved ? "translate-x-4" : "translate-x-1"
                }`}
              />
            </button>
          )}
        </div>
      </div>

      {/* Comments */}
      {(approval.comments.length > 0 || canAct) && (
        <SubCardBody>
          {approval.comments.length > 0 && (
            <ul className="space-y-2">
              {approval.comments.map((c) => (
                <li key={c.id} className="rounded-md border bg-white px-3 py-2 text-sm shadow-sm">
                  <div className="flex items-baseline justify-between gap-2">
                    <span className="font-medium text-gray-700">
                      {reviewerLabel(c.author?.name, c.author?.email, c.author_id)}
                    </span>
                    <span className="shrink-0 text-[11px] text-gray-400">
                      {new Date(c.created_at).toLocaleString()}
                    </span>
                  </div>
                  {c.comment && <p className="mt-1 whitespace-pre-wrap text-gray-700">{c.comment}</p>}
                  {c.documents.length > 0 && (
                    <div className="mt-1.5 flex flex-wrap gap-1.5">
                      {c.documents.map((d: Document) => (
                        <button
                          key={d.id}
                          type="button"
                          className="inline-flex items-center gap-1 rounded-full border bg-gray-50 px-2 py-0.5 text-xs text-indigo-600 hover:bg-gray-100"
                          onClick={() => downloadRecCommentDocument(prId, c.id, d)}
                        >
                          <span aria-hidden>📎</span>
                          {d.filename}
                        </button>
                      ))}
                    </div>
                  )}
                </li>
              ))}
            </ul>
          )}

          {canAct &&
            (open ? (
              <div className="mt-3 space-y-2">
                <textarea
                  className={inputCls}
                  rows={2}
                  value={comment}
                  onChange={(e) => setComment(e.target.value)}
                  placeholder="Comment"
                />
                <div>
                  <label className="cursor-pointer text-sm text-indigo-600">
                    + Attach documents
                    <input
                      type="file"
                      multiple
                      className="hidden"
                      onChange={(e) => setFiles(Array.from(e.target.files ?? []))}
                    />
                  </label>
                  {files.length > 0 && (
                    <ul className="mt-1 text-xs text-gray-600">
                      {files.map((f, i) => (
                        <li key={i}>{f.name}</li>
                      ))}
                    </ul>
                  )}
                </div>
                {error && <p className="text-sm text-red-600">{error}</p>}
                <div className="flex gap-2">
                  <button
                    type="button"
                    disabled={commentMutation.isPending || (comment.trim() === "" && files.length === 0)}
                    onClick={() => commentMutation.mutate()}
                    className="rounded bg-indigo-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-indigo-700 disabled:opacity-50"
                  >
                    {commentMutation.isPending ? "Saving…" : "Add comment"}
                  </button>
                  <button
                    type="button"
                    onClick={() => {
                      setOpen(false);
                      setComment("");
                      setFiles([]);
                    }}
                    className="rounded px-3 py-1.5 text-sm text-gray-600 hover:bg-gray-50"
                  >
                    Cancel
                  </button>
                </div>
              </div>
            ) : (
              <button
                type="button"
                onClick={() => setOpen(true)}
                className={`text-sm text-indigo-600 hover:underline ${approval.comments.length > 0 ? "mt-3" : ""}`}
              >
                + Add comment
              </button>
            ))}
        </SubCardBody>
      )}
    </SubCard>
  );
}
