import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  CircularProgress,
  Link as MuiLink,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { FileText } from "@wso2/oxygen-ui-icons-react";
import { useQuotation } from "../hooks/useQuotations";
import { useProcurementAccess } from "../hooks/useProcurementAccess";
import { EntityStatusBadge } from "../components/EntityStatusBadge";
import { QuotationFields } from "../components/QuotationFields";
import { DocumentList } from "../components/DocumentList";
import { useConfirmAction } from "../components/ConfirmDialog";
import { RelatedEntities } from "../components/RelatedEntities";
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
import type { QuotationPdfSlot } from "../api/quotations";
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
    mutationFn: ({ slot, file }: { slot: QuotationPdfSlot; file: File }) =>
      uploadQuotationPDF(quoId, slot, file),
    onSuccess: invalidate,
    onError: (e) => setActionError(e instanceof ApiError ? e.message : "Upload failed"),
  });

  const deletePdfMutation = useMutation({
    mutationFn: (slot: QuotationPdfSlot) => deleteQuotationPDF(quoId, slot),
    onSuccess: invalidate,
    onError: (e) => setActionError(e instanceof ApiError ? e.message : "Delete failed"),
  });

  const onPickPdf = (slot: QuotationPdfSlot, file: File | null) => {
    if (!file) return;
    if (!/\.pdf$/i.test(file.name)) {
      setActionError(`Only .pdf files are allowed (got ${file.name}).`);
      return;
    }
    setActionError(null);
    uploadPdfMutation.mutate({ slot, file });
  };

  if (isLoading)
    return (
      <Box sx={{ maxWidth: 1200, mx: "auto", p: { xs: 2, md: 4 } }}>
        <CircularProgress size={24} />
      </Box>
    );
  if (error || !q)
    return (
      <Box sx={{ maxWidth: 1200, mx: "auto", p: { xs: 2, md: 4 } }}>
        <Alert severity="error">Failed to load quotation.</Alert>
      </Box>
    );

  return (
    <Box sx={{ maxWidth: 1200, mx: "auto", p: { xs: 2, md: 4 } }}>
      <Stack direction="row" spacing={1} alignItems="center" sx={{ mb: 2 }}>
        <MuiLink component={Link} to="/quotations" variant="body2">
          Quotations
        </MuiLink>
        <Typography variant="body2" color="text.secondary">
          /
        </Typography>
        <Typography variant="body2" color="text.secondary">
          {quoRef(q.id)}
        </Typography>
      </Stack>

      <ChainStepper prId={q.purchase_request_id} current={{ kind: "quotation", id: q.id }} />

      <Stack
        direction="row"
        justifyContent="space-between"
        alignItems="flex-start"
        sx={{ mb: 2 }}
      >
        <Box>
          <Typography variant="h5" sx={{ fontWeight: 600 }}>
            {q.vendor?.name ?? `Vendor #${q.vendor_id}`}
          </Typography>
          <Box sx={{ mt: 1 }}>
            <EntityStatusBadge status={q.status} />
          </Box>
        </Box>
        {procurement && !editing && (
          <Stack direction="row" spacing={1}>
            <Button
              variant="outlined"
              color="inherit"
              onClick={() => {
                setDraft(toInput(q));
                setEditing(true);
              }}
            >
              Edit
            </Button>
            {q.status !== "selected" && (
              <Button
                variant="outlined"
                color="inherit"
                onClick={() => selectMutation.mutate()}
                disabled={selectMutation.isPending}
              >
                Select
              </Button>
            )}
          </Stack>
        )}
      </Stack>

      {actionError && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {actionError}
        </Alert>
      )}

      <Card variant="outlined">
        <CardContent>
          {editing && draft ? (
            <>
              <QuotationFields value={draft} onChange={setDraft} />
              <Stack direction="row" spacing={1} sx={{ mt: 3 }}>
                <Button
                  variant="contained"
                  onClick={() => saveMutation.mutate()}
                  disabled={saveMutation.isPending}
                >
                  {saveMutation.isPending ? "Saving…" : "Save changes"}
                </Button>
                <Button
                  variant="outlined"
                  color="inherit"
                  onClick={() => {
                    setEditing(false);
                    setActionError(null);
                  }}
                >
                  Cancel
                </Button>
              </Stack>
            </>
          ) : (
            <Stack spacing={2}>
              <Field label="Total">{formatMoney(q.total_amount, q.currency)}</Field>
              <Field label="Valid until">{q.valid_until ?? "—"}</Field>
              <Field label="Line items">
                {q.items.length === 0 ? (
                  "—"
                ) : (
                  <Box component="ul" sx={{ listStyle: "disc", pl: 2.5, m: 0 }}>
                    {q.items.map((it) => (
                      <li key={it.id}>
                        {it.description} —{" "}
                        <Box component="span" sx={{ color: "text.secondary" }}>
                          qty {it.quantity} × {formatMoney(it.unit_price, q.currency)}
                        </Box>
                      </li>
                    ))}
                  </Box>
                )}
              </Field>
              <Field label="Notes">
                <Box component="span" sx={{ whiteSpace: "pre-wrap" }}>
                  {q.notes || "—"}
                </Box>
              </Field>
            </Stack>
          )}
        </CardContent>
      </Card>

      <DirectParentCard prId={q.purchase_request_id} current={{ kind: "quotation", id: q.id }} />

      <QuotationPdfCard
        title="Initial quotation"
        slot="initial"
        doc={q.initial_quotation_document ?? null}
        quotationId={q.id}
        canEdit={procurement}
        onPick={onPickPdf}
        onRemove={(slot) => deletePdfMutation.mutate(slot)}
      />
      <QuotationPdfCard
        title="Final quotation"
        slot="final"
        doc={q.final_quotation_document ?? null}
        quotationId={q.id}
        canEdit={procurement}
        onPick={onPickPdf}
        onRemove={(slot) => deletePdfMutation.mutate(slot)}
      />

      <Box sx={{ mt: 3 }}>
        <DocumentList
          documents={q.documents ?? []}
          canEdit={procurement}
          title="Other documents"
          onUpload={(f) => uploadMutation.mutate(f)}
          onDelete={(docId) => deleteMutation.mutate(docId)}
          onDownload={(doc: Document) => downloadQuotationDocument(q.id, doc)}
          onError={setActionError}
        />
      </Box>

      <RelatedEntities prId={q.purchase_request_id} current={{ kind: "quotation", id: q.id }} />
    </Box>
  );
}

// QuotationPdfCard is one of the quotation's two primary PDF slots — the initial
// quote from the vendor and the final (post-negotiation) one. Both are optional
// and independently replaceable/removable; the same pair is editable from the
// quotation's card on the PR page.
function QuotationPdfCard({
  title,
  slot,
  doc,
  quotationId,
  canEdit,
  onPick,
  onRemove,
}: {
  title: string;
  slot: QuotationPdfSlot;
  doc: Document | null;
  quotationId: number;
  canEdit: boolean;
  onPick: (slot: QuotationPdfSlot, file: File | null) => void;
  onRemove: (slot: QuotationPdfSlot) => void;
}) {
  const [confirmNode, confirmRemove] = useConfirmAction();

  return (
    <Card variant="outlined" sx={{ mt: 3 }}>
      <CardContent>
        <Stack direction="row" justifyContent="space-between" alignItems="center" sx={{ mb: 1.5 }}>
          <Typography variant="subtitle1" sx={{ fontWeight: 600 }}>
            {title}
          </Typography>
          {canEdit && (
            <Button variant="text" component="label">
              {doc ? "Replace" : "+ Add PDF"}
              <input
                type="file"
                accept=".pdf"
                hidden
                onChange={(e) => {
                  onPick(slot, e.target.files?.[0] ?? null);
                  e.target.value = "";
                }}
              />
            </Button>
          )}
        </Stack>
        {doc ? (
          <Stack direction="row" justifyContent="space-between" alignItems="center">
            <MuiLink
              component="button"
              type="button"
              variant="body2"
              onClick={() => downloadQuotationDocument(quotationId, doc)}
              sx={{ display: "inline-flex", alignItems: "center", gap: 0.5 }}
            >
              <FileText size={16} />
              {doc.filename}
            </MuiLink>
            <Stack direction="row" spacing={1.5} alignItems="center">
              <Typography variant="caption" color="text.secondary">
                {(doc.size_bytes / 1024).toFixed(0)} KB
              </Typography>
              {canEdit && (
                <Button
                  variant="text"
                  color="error"
                  size="small"
                  onClick={() =>
                    confirmRemove({
                      title: `Remove ${title.toLowerCase()} PDF`,
                      message: (
                        <>
                          Remove <strong>{doc.filename}</strong>? The file is deleted, along with
                          anything read from it. The quotation's stored figures are left as they are.
                        </>
                      ),
                      confirmLabel: "Remove",
                      onConfirm: () => onRemove(slot),
                    })
                  }
                >
                  Remove
                </Button>
              )}
            </Stack>
          </Stack>
        ) : (
          <Typography variant="body2" color="text.secondary">
            No {title.toLowerCase()} PDF attached.
          </Typography>
        )}
      </CardContent>
      {confirmNode}
    </Card>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <Box>
      <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 600 }}>
        {label}
      </Typography>
      <Typography variant="body2" component="div">
        {children}
      </Typography>
    </Box>
  );
}
