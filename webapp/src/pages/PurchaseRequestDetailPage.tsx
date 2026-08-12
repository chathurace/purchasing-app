import { useMemo, useRef, useState } from "react";
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
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { FileText, Plus, Sparkles } from "@wso2/oxygen-ui-icons-react";
import { usePurchaseRequest } from "../hooks/usePurchaseRequests";
import { useBusinessUnitLookup } from "../hooks/useBusinessUnits";
import { useMe } from "../hooks/useMe";
import { useProcurementAccess } from "../hooks/useProcurementAccess";
import { useQuotationsForPR } from "../hooks/useQuotations";
import { StatusBadge } from "../components/StatusBadge";
import { EntityStatusBadge } from "../components/EntityStatusBadge";
import { RequisitionForm, requisitionTitle } from "../components/RequisitionForm";
import { TeamLeadApprovalCard } from "../components/TeamLeadApprovalCard";
import { AssignmentCard } from "../components/AssignmentCard";
import { RelatedEntities } from "../components/RelatedEntities";
import { QuotationExtractionReview } from "../components/QuotationExtractionReview";
import { ExtractedQuotationDetails } from "../components/ExtractedQuotationDetails";
import { useConfirmAction } from "../components/ConfirmDialog";
import {
  QuotationComparisonButton,
  QuotationComparisonCard,
} from "../components/QuotationComparisonCard";
import { comparisonKey } from "../hooks/useQuotationComparison";
import { ChainStepper } from "../components/ChainStepper";
import { VendorSelect } from "../components/VendorSelect";
import { RecommendationSection } from "../components/RecommendationSection";
import { rejectPurchaseRequest, updatePurchaseRequest } from "../api/purchaseRequests";
import {
  createQuotation,
  deleteQuotationPDF,
  downloadQuotationDocument,
  updateQuotation,
  uploadQuotationPDF,
} from "../api/quotations";
import type { QuotationPdfSlot } from "../api/quotations";
import { extractForPR, extractForQuotation } from "../api/quotationExtractions";
import {
  useCanExtractQuotations,
  usePRExtractions,
} from "../hooks/useQuotationExtraction";
import { ApiError } from "../api/client";
import { EDITABLE_STATUSES, formatMoney, prReference, quoRef } from "../types/api";
import type {
  Document,
  ExtractionResponse,
  PurchaseRequest,
  PurchaseRequestInput,
  Quotation,
  QuotationInput,
} from "../types/api";

function toInput(pr: PurchaseRequest): PurchaseRequestInput {
  return {
    title: pr.title,
    business_unit_id: pr.business_unit_id,
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

  if (isLoading)
    return (
      <Box sx={{ maxWidth: 1200, mx: "auto", p: { xs: 2, md: 4 } }}>
        <CircularProgress size={24} />
      </Box>
    );
  if (error || !pr)
    return (
      <Box sx={{ maxWidth: 1200, mx: "auto", p: { xs: 2, md: 4 } }}>
        <Alert severity="error">Failed to load request.</Alert>
      </Box>
    );

  return (
    <Box sx={{ maxWidth: 1200, mx: "auto", p: { xs: 2, md: 4 } }}>
      <Stack direction="row" spacing={1} alignItems="center" sx={{ mb: 2 }}>
        <MuiLink component={Link} to={procurement ? "/requests" : "/my-requests"} variant="body2">
          {procurement ? "Purchase requests" : "My requests"}
        </MuiLink>
        <Typography variant="body2" color="text.secondary">
          /
        </Typography>
        <Typography variant="body2" color="text.secondary">
          {prReference(pr)}
        </Typography>
      </Stack>

      <ChainStepper prId={pr.id} current={{ kind: "pr", id: pr.id }} />

      <Stack
        direction="row"
        justifyContent="space-between"
        alignItems="flex-start"
        gap={2}
        sx={{ mb: 2 }}
      >
        <Box>
          <Typography variant="h5" sx={{ fontWeight: 600 }}>
            {pr.title || prReference(pr)}
          </Typography>
          <Box sx={{ mt: 1 }}>
            <StatusBadge status={pr.status} />
          </Box>
        </Box>
        {canEdit &&
          (editing ? (
            <Button
              variant="outlined"
              color="inherit"
              onClick={() => {
                setEditing(false);
                setActionError(null);
              }}
            >
              Cancel editing
            </Button>
          ) : (
            <Button variant="outlined" color="inherit" onClick={startEditing}>
              Edit details
            </Button>
          ))}
      </Stack>

      {!canEdit && (
        <Alert severity="info" sx={{ mb: 2 }}>
          This request is read-only in its current state.
        </Alert>
      )}

      {actionError && !editing && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {actionError}
        </Alert>
      )}

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
        <Card variant="outlined">
          <CardContent>
            <ReadOnlyView pr={pr} />
          </CardContent>
        </Card>
      )}

      <TeamLeadApprovalCard pr={pr} me={me} />

      <AssignmentCard pr={pr} me={me} />

      {procurement && pr.my_can_work && <ProcurementSection pr={pr} />}

      {/* Rendered at page level, not inside ProcurementSection: the comparison is
          what the recommendation's approvers read, and they have no procurement
          access. It renders nothing until procurement has generated one. */}
      <QuotationComparisonCard prId={pr.id} />

      <RecommendationSection pr={pr} />

      {/* Quotations are excluded by the case graph (they are this page's direct
          children); the recommendation's contract has its own card above, so it
          is excluded here too — leaving the fulfillment records (GRNs, invoices)
          and any contract not on the recommendation. */}
      <RelatedEntities
        prId={pr.id}
        current={{ kind: "pr", id: pr.id }}
        exclude={
          pr.recommendation?.contract_id != null
            ? [{ kind: "contract", id: pr.recommendation.contract_id }]
            : []
        }
      />
    </Box>
  );
}

// QuotationCard renders a single quotation on the PR page as a card: vendor,
// description and the two primary PDF slots (initial + final), each of which carries
// its own extracted figures. There is no summary expander: the totals, validity and
// line items now live in the slot that they were read from — the initial and final
// PDFs disagree, so a single card-level summary could only show one of them. The
// header links to the quotation's own page for the stored record (other documents).
// The final PDF is uploaded from here after the quotation exists — it is optional and
// does not gate selecting the quotation for the recommendation.
function QuotationCard({ q, canEdit }: { q: Quotation; canEdit: boolean }) {
  const [error, setError] = useState<string | null>(null);
  // review holds a read of one PDF awaiting the user's approval, tagged with the slot
  // it came from so the editable panel renders inside that slot's block rather than
  // floating at the bottom of the card.
  const [review, setReview] = useState<{
    slot: QuotationPdfSlot;
    result: ExtractionResponse;
  } | null>(null);
  const canExtract = useCanExtractQuotations();
  const qc = useQueryClient();

  // What each of this PR's quotation PDFs said, looked up per slot by document id.
  // The query key is per-PR, so every card on the page shares one request.
  const { data: prExtractions } = usePRExtractions(
    q.purchase_request_id,
    canExtract,
  );
  const extractedFor = (docID: number | null | undefined) =>
    docID == null
      ? null
      : ((prExtractions ?? []).find(
          (e) => e.extraction.document_id === docID,
        ) ?? null);

  const invalidate = () => {
    qc.invalidateQueries({
      queryKey: ["quotations", "pr", q.purchase_request_id],
    });
    qc.invalidateQueries({ queryKey: ["quotations", q.id] });
    qc.invalidateQueries({ queryKey: ["quotations"] });
    qc.invalidateQueries({
      queryKey: ["purchase-requests", q.purchase_request_id, "related"],
    });
    qc.invalidateQueries({
      queryKey: ["quotation-extractions", "pr", q.purchase_request_id],
    });
    // The comparison is derived from these figures, so a changed quotation refreshes
    // it immediately rather than on its next poll.
    qc.invalidateQueries({ queryKey: comparisonKey(q.purchase_request_id) });
  };

  // Uploading a PDF reads it straight away and opens the editable review in the same
  // slot: the point of attaching a quotation PDF is the numbers in it, so making the
  // user click "Read details" as a second step was busywork. The read is still only a
  // suggestion — nothing reaches the quotation until they approve it.
  const uploadMutation = useMutation({
    mutationFn: ({ slot, file }: { slot: QuotationPdfSlot; file: File }) =>
      uploadQuotationPDF(q.id, slot, file),
    onSuccess: (_res, { slot }) => {
      invalidate();
      setError(null);
      if (canExtract && canEdit) extractMutation.mutate(slot);
    },
    onError: (e) =>
      setError(e instanceof ApiError ? e.message : "Upload failed"),
  });

  const removeMutation = useMutation({
    mutationFn: (slot: QuotationPdfSlot) => deleteQuotationPDF(q.id, slot),
    onSuccess: (_res, slot) => {
      invalidate();
      setError(null);
      // The PDF the review belongs to is gone; the review would be reviewing nothing.
      setReview((r) => (r?.slot === slot ? null : r));
    },
    onError: (e) =>
      setError(e instanceof ApiError ? e.message : "Failed to remove the PDF"),
  });

  // extractMutation reads an attached PDF; applyMutation writes the reviewed values
  // onto the quotation. Nothing is applied without the user confirming.
  const extractMutation = useMutation({
    mutationFn: (slot: QuotationPdfSlot) => extractForQuotation(q.id, slot),
    onSuccess: (res, slot) => {
      setReview({ slot, result: res });
      setError(null);
      // The result is stored, so the per-slot panel shows it whether or not the user
      // goes on to apply it to the quotation.
      qc.invalidateQueries({
        queryKey: ["quotation-extractions", "pr", q.purchase_request_id],
      });
    },
    onError: (e) =>
      setError(
        e instanceof ApiError
          ? e.message
          : "Could not read details from that PDF",
      ),
  });

  const applyMutation = useMutation({
    mutationFn: (input: QuotationInput) => updateQuotation(q.id, input),
    onSuccess: () => {
      invalidate();
      setReview(null);
      setError(null);
    },
    onError: (e) =>
      setError(
        e instanceof ApiError ? e.message : "Failed to apply the details",
      ),
  });

  const onPick = (slot: QuotationPdfSlot, file: File | null) => {
    if (!file) return;
    if (!/\.pdf$/i.test(file.name)) {
      setError(`Only .pdf files are allowed (got ${file.name}).`);
      return;
    }
    setError(null);
    uploadMutation.mutate({ slot, file });
  };

  const busy =
    uploadMutation.isPending ||
    removeMutation.isPending ||
    extractMutation.isPending;

  return (
    <Card
      component="li"
      variant="outlined"
      sx={{ listStyle: "none", borderRadius: 2 }}
    >
      <CardContent>
        <Stack spacing={1.5}>
          <Stack
            direction="row"
            justifyContent="space-between"
            alignItems="flex-start"
            gap={1.5}
          >
            <Box sx={{ minWidth: 0 }}>
              {/* Links to the quotation's own page — the two slot panels cover the
                  figures, but the stored record (other documents, full line items)
                  lives there. */}
              <MuiLink
                component={Link}
                to={`/quotations/${q.id}`}
                variant="body2"
                sx={{ fontWeight: 600 }}
              >
                {quoRef(q.id)} — {q.vendor?.name ?? `Vendor #${q.vendor_id}`}
              </MuiLink>
              {q.notes ? (
                <Typography
                  variant="body2"
                  color="text.secondary"
                  sx={{ whiteSpace: "pre-wrap" }}
                >
                  {q.notes}
                </Typography>
              ) : (
                <Typography variant="body2" color="text.secondary">
                  No description.
                </Typography>
              )}
            </Box>
            <EntityStatusBadge status={q.status} />
          </Stack>

          {error && <Alert severity="error">{error}</Alert>}

          <Stack spacing={1}>
            {(["initial", "final"] as QuotationPdfSlot[]).map((slot) => {
              const doc =
                (slot === "initial"
                  ? q.initial_quotation_document
                  : q.final_quotation_document) ?? null;
              const extracted = extractedFor(doc?.id);
              return (
                <QuotationPdfSlotRow
                  key={slot}
                  label={slot === "initial" ? "Initial quotation" : "Final quotation"}
                  slot={slot}
                  doc={doc}
                  quotationId={q.id}
                  canEdit={canEdit}
                  busy={busy}
                  onPick={onPick}
                  onRemove={(s) => removeMutation.mutate(s)}
                  onExtract={
                    canExtract && canEdit
                      ? () => extractMutation.mutate(slot)
                      : undefined
                  }
                  extracting={
                    extractMutation.isPending && extractMutation.variables === slot
                  }
                  extracted={extracted}
                  review={
                    review?.slot === slot ? (
                      <QuotationExtractionReview
                        result={review.result}
                        submitLabel="Apply to quotation"
                        submitting={applyMutation.isPending}
                        initialNotes={q.notes}
                        onSubmit={(input) => applyMutation.mutate(input)}
                        onDiscard={() => setReview(null)}
                      />
                    ) : null
                  }
                />
              );
            })}
          </Stack>

        </Stack>
      </CardContent>
    </Card>
  );
}

// QuotationPdfSlotRow renders one of a quotation's two primary PDF slots: the
// attached file (downloadable) with Replace/Remove, or an Upload button when the
// slot is empty.
function QuotationPdfSlotRow({
  label,
  slot,
  doc,
  quotationId,
  canEdit,
  busy,
  onPick,
  onRemove,
  onExtract,
  extracting,
  extracted,
  review,
}: {
  label: string;
  slot: QuotationPdfSlot;
  doc: Document | null;
  quotationId: number;
  canEdit: boolean;
  busy: boolean;
  onPick: (slot: QuotationPdfSlot, file: File | null) => void;
  onRemove: (slot: QuotationPdfSlot) => void;
  /** Re-read this PDF with Claude. Omitted when extraction is unavailable. */
  onExtract?: () => void;
  extracting?: boolean;
  /**
   * What was read out of *this* PDF, if it has been read. Per-slot on purpose: the
   * initial and final quotation are separate documents whose figures and line items
   * legitimately differ.
   */
  extracted?: ExtractionResponse | null;
  /** The editable review panel, when this slot's PDF is awaiting approval. */
  review?: React.ReactNode;
}) {
  // Already read = nothing to gain from spending another call on the same bytes.
  // Uploading a replacement makes a new document, which has no extraction, so the
  // button comes back by itself.
  const alreadyRead = extracted != null || review != null;
  const [confirmNode, confirmRemove] = useConfirmAction();
  return (
    <Stack
      spacing={1}
      sx={{ p: 1, border: 1, borderColor: "divider", borderRadius: 1.5 }}
    >
      <Stack
        direction={{ xs: "column", sm: "row" }}
        justifyContent="space-between"
        alignItems={{ xs: "flex-start", sm: "center" }}
        gap={0.5}
      >
        <Box sx={{ minWidth: 0 }}>
          <Typography
            variant="caption"
            color="text.secondary"
            sx={{ fontWeight: 600, display: "block" }}
          >
            {label}
          </Typography>
          {doc ? (
            <MuiLink
              component="button"
              type="button"
              variant="body2"
              onClick={() => downloadQuotationDocument(quotationId, doc)}
              sx={{ display: "inline-flex", alignItems: "center", gap: 0.5 }}
            >
              <FileText size={14} />
              {doc.filename}
            </MuiLink>
          ) : (
            <Typography variant="body2" color="text.secondary">
              Not uploaded.
            </Typography>
          )}
        </Box>
        {canEdit && (
          <Stack
            direction="row"
            spacing={0.5}
            alignItems="center"
            sx={{ flexShrink: 0 }}
          >
            <Button
              variant="text"
              size="small"
              component="label"
              disabled={busy}
            >
              {doc ? "Replace" : "Upload"}
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
            {doc && onExtract && (
              <Button
                variant="text"
                size="small"
                startIcon={<Sparkles size={14} />}
                disabled={busy || alreadyRead}
                onClick={onExtract}
                title={
                  alreadyRead
                    ? "This PDF has already been read — upload a new one to read again"
                    : "Read the vendor, total, taxes and line items from this PDF"
                }
              >
                {extracting ? "Reading…" : alreadyRead ? "Read" : "Read details"}
              </Button>
            )}
            {doc && (
              <Button
                variant="text"
                color="error"
                size="small"
                disabled={busy}
                onClick={() =>
                  confirmRemove({
                    title: `Remove ${label.toLowerCase()}`,
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
        )}
      </Stack>
      {/* While a read is awaiting approval the editable panel replaces the read-only
          one: two views of the same PDF side by side would just invite confusion
          about which figures are live. */}
      {review}
      {!review && doc && extracted?.suggestion && (
        <ExtractedQuotationDetails result={extracted} />
      )}
      {confirmNode}
    </Stack>
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
  // extraction holds the reviewable result of reading the chosen PDF; while set,
  // the review panel replaces the bare vendor+description form.
  const [extraction, setExtraction] = useState<ExtractionResponse | null>(null);
  const canExtract = useCanExtractQuotations();

  const closed = ["rejected", "cancelled", "order_signed", "completed"].includes(pr.status);

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ["quotations", "pr", pr.id] });
    qc.invalidateQueries({ queryKey: ["quotations"] });
    qc.invalidateQueries({ queryKey: ["purchase-requests", pr.id] });
    qc.invalidateQueries({ queryKey: ["purchase-requests"] });
    qc.invalidateQueries({ queryKey: ["purchase-requests", pr.id, "related"] });
    // A created-from-extraction quotation adopts the staged PDF, which moves the
    // extraction onto the new quotation's initial slot.
    qc.invalidateQueries({ queryKey: ["quotation-extractions", "pr", pr.id] });
    // A new (or removed) quotation changes what the comparison compares.
    qc.invalidateQueries({ queryKey: comparisonKey(pr.id) });
  };

  const resetForm = () => {
    setVendorId(0);
    setDescription("");
    setFile(null);
    setExtraction(null);
    if (fileRef.current) fileRef.current.value = "";
    setError(null);
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
      // Only the initial quotation can be attached at creation; the final one is
      // uploaded later from the quotation's card.
      if (file) await uploadQuotationPDF(q.id, "initial", file);
      return q;
    },
    onSuccess: () => {
      invalidate();
      resetForm();
    },
    onError: (e) =>
      setError(e instanceof ApiError ? e.message : "Failed to add quotation"),
  });

  // extractMutation reads the chosen PDF before the quotation exists, so the form
  // can be pre-filled from it. The PDF is stored server-side as part of this call
  // and adopted as the quotation's initial document on create (via extraction_id),
  // so it is never uploaded twice.
  const extractMutation = useMutation({
    mutationFn: (f: File) => extractForPR(pr.id, f),
    onSuccess: (res) => {
      setExtraction(res);
      setError(null);
    },
    onError: (e) =>
      setError(
        e instanceof ApiError
          ? e.message
          : "Could not read details from that PDF",
      ),
  });

  // createFromExtraction writes the reviewed values in one call. The PDF is already
  // stored, so there is no follow-up upload.
  const createFromExtraction = useMutation({
    mutationFn: (input: QuotationInput) => createQuotation(pr.id, input),
    onSuccess: () => {
      invalidate();
      resetForm();
    },
    onError: (e) =>
      setError(e instanceof ApiError ? e.message : "Failed to add quotation"),
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
    <Card variant="outlined" sx={{ mt: 3 }}>
      <CardContent>
        <Stack direction="row" justifyContent="space-between" alignItems="center" sx={{ mb: 1.5 }}>
          <Typography variant="subtitle1" sx={{ fontWeight: 600 }}>
            Quotations
          </Typography>
          <Stack direction="row" spacing={1} alignItems="center">
            {/* Offered once two or more vendors have quoted — the comparison card
                itself renders below, where every viewer of the PR can read it. */}
            <QuotationComparisonButton pr={pr} quotationCount={quotations?.length ?? 0} />
            {!closed && !showReject && (
              <Button variant="text" color="error" size="small" onClick={() => setShowReject(true)}>
                Reject request
              </Button>
            )}
          </Stack>
        </Stack>

        {error && (
          <Alert severity="error" sx={{ mb: 1.5 }}>
            {error}
          </Alert>
        )}

        {showReject && (
          <Box
            sx={{
              mb: 2,
              p: 2,
              border: 1,
              borderColor: "error.main",
              borderRadius: 2,
              bgcolor: "action.hover",
            }}
          >
            <Stack spacing={1.5}>
              <TextField
                label="Rejection comment"
                size="small"
                fullWidth
                multiline
                minRows={2}
                value={rejectComment}
                onChange={(e) => setRejectComment(e.target.value)}
                placeholder="Why is this request being rejected?"
              />
              <Stack direction="row" spacing={1}>
                <Button
                  variant="contained"
                  color="error"
                  size="small"
                  disabled={rejectMutation.isPending || rejectComment.trim() === ""}
                  onClick={() => rejectMutation.mutate()}
                >
                  {rejectMutation.isPending ? "Rejecting…" : "Confirm rejection"}
                </Button>
                <Button
                  variant="outlined"
                  color="inherit"
                  size="small"
                  onClick={() => {
                    setShowReject(false);
                    setError(null);
                  }}
                >
                  Cancel
                </Button>
              </Stack>
            </Stack>
          </Box>
        )}

        {quotations && quotations.length > 0 ? (
          <Stack
            component="ul"
            spacing={1.5}
            sx={{ listStyle: "none", p: 0, m: 0 }}
          >
            {quotations.map((q) => (
              <QuotationCard key={q.id} q={q} canEdit={!closed} />
            ))}
          </Stack>
        ) : (
          <Typography variant="body2" color="text.secondary">
            No quotations yet.
          </Typography>
        )}

        {!closed && (
          <Box
            sx={{
              mt: 3,
              p: 2.5,
              border: 1,
              borderColor: "primary.main",
              borderRadius: 2,
              bgcolor: "action.hover",
            }}
          >
            <Stack spacing={1.5}>
              <Stack direction="row" spacing={1} alignItems="center">
                <Box
                  sx={{
                    width: 28,
                    height: 28,
                    borderRadius: "50%",
                    bgcolor: "primary.main",
                    color: "primary.contrastText",
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "center",
                    flexShrink: 0,
                  }}
                >
                  <Plus size={16} />
                </Box>
                <Box>
                  <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>
                    Add a quotation
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    {canExtract
                      ? "Attach the vendor's quotation PDF and its details will be read automatically for you to check. The final quotation can be uploaded on the card afterwards."
                      : "Record a vendor's quote against this request and attach the initial quotation. The final quotation can be uploaded on the card afterwards."}
                  </Typography>
                </Box>
              </Stack>

              {extraction ? (
                // Reviewing an extracted PDF: the panel owns the whole form, since
                // every field it collects came from (or corrects) the document.
                <QuotationExtractionReview
                  result={extraction}
                  submitLabel="Add quotation"
                  submitting={createFromExtraction.isPending}
                  initialNotes={description}
                  onSubmit={(input) => createFromExtraction.mutate(input)}
                  onDiscard={resetForm}
                />
              ) : (
                <>
                  <VendorSelect value={vendorId} onChange={setVendorId} />
                  <TextField
                    size="small"
                    fullWidth
                    multiline
                    minRows={2}
                    value={description}
                    onChange={(e) => setDescription(e.target.value)}
                    placeholder="Description (optional)"
                  />
                  <Box>
                    <Typography
                      variant="body2"
                      sx={{ fontWeight: 500, mb: 0.5 }}
                    >
                      Initial quotation PDF (optional)
                    </Typography>
                    <Stack direction="row" spacing={1} alignItems="center">
                      <Button
                        variant="outlined"
                        color="inherit"
                        size="small"
                        component="label"
                      >
                        {file ? file.name : "Choose PDF"}
                        <input
                          ref={fileRef}
                          type="file"
                          accept=".pdf"
                          hidden
                          onChange={(e) =>
                            onPickFile(e.target.files?.[0] ?? null)
                          }
                        />
                      </Button>
                      {canExtract && file && (
                        <Button
                          variant="outlined"
                          size="small"
                          startIcon={<Sparkles size={14} />}
                          disabled={extractMutation.isPending}
                          onClick={() => extractMutation.mutate(file)}
                        >
                          {extractMutation.isPending
                            ? "Reading PDF…"
                            : "Read details from PDF"}
                        </Button>
                      )}
                    </Stack>
                    {canExtract && file && !extractMutation.isPending && (
                      <Typography
                        variant="caption"
                        color="text.secondary"
                        sx={{ mt: 0.5, display: "block" }}
                      >
                        Optional — reads the vendor, total, currency and line
                        items so you don't have to retype them. You review
                        everything before it's saved.
                      </Typography>
                    )}
                  </Box>
                  <Button
                    variant="contained"
                    fullWidth
                    disabled={createMutation.isPending || !vendorId}
                    onClick={() => createMutation.mutate()}
                  >
                    {createMutation.isPending ? "Adding…" : "Add quotation"}
                  </Button>
                </>
              )}
            </Stack>
          </Box>
        )}
      </CardContent>
    </Card>
  );
}

function yesNo(v?: string): string {
  return v === "yes" ? "Yes" : v === "no" ? "No" : v === "unknown" ? "Don't know" : "";
}

function ReadOnlyView({ pr }: { pr: PurchaseRequest }) {
  const d = pr.details ?? {};
  const { data: businessUnits } = useBusinessUnitLookup();
  const bu = (businessUnits ?? []).find((c) => c.id === pr.business_unit_id);
  const buLabel = bu ? bu.name : pr.business_unit_id ? `#${pr.business_unit_id}` : "";
  return (
    <Stack spacing={1.5}>
      <Section title="Requester" defaultOpen>
        <Row label="Reference" value={prReference(pr)} />
        <Row label="Requester" value={d.requester_name || pr.requester?.email || `#${pr.requester_id}`} />
        <Row label="Email" value={d.requester_email || pr.requester?.email} />
        <Row label="Date" value={d.date} />
        <Row label="Business unit" value={buLabel} />
        <Row label="WSO2 entity" value={pr.entity} />
        <Row label="Business justification" value={d.business_justification} pre />
      </Section>

      <Section title="Purchase">
        <Row
          label="Type"
          value={pr.category === "IT" ? "IT solution" : pr.category === "NON-IT" ? "Non-IT solution" : pr.category === "EVENTS" ? "Marketing & Events" : ""}
        />
        {pr.category === "IT" && (
          <>
            <Row label="IT category" value={d.it_category} />
            <Row label="Product / solution" value={d.it_product} />
            <Row label="Description" value={d.it_description} pre />
            <Row label="Plan / tier" value={d.it_plan} />
            <Row label="Users" value={[d.it_user_count, (d.it_user_names ?? []).filter(Boolean).join(", ")].filter(Boolean).join(" — ") || d.it_users} />
            <Row label="Administrators" value={[d.it_admin_count, (d.it_admin_names ?? []).filter(Boolean).join(", ")].filter(Boolean).join(" — ") || d.it_admins} />
            <Row label="Expected usage period" value={d.it_usage} />
            <Row label="Stores sensitive data" value={yesNo(d.sec_sensitive)} />
            <Row label="Captures external PII" value={yesNo(d.sec_external_pii)} />
            {d.sec_external_pii === "yes" && <Row label="External PII detail" value={d.sec_external_pii_detail} />}
            <Row label="Captures employee PII" value={yesNo(d.sec_employee_pii)} />
            {d.sec_employee_pii === "yes" && <Row label="Employee PII detail" value={d.sec_employee_pii_detail} />}
            <Row label="Integrates with internal systems" value={yesNo(d.sec_integrates)} />
            {d.sec_integrates === "yes" && (
              <>
                <Row label="Integration systems" value={d.sec_integration_systems} />
                <Row label="Integration type" value={d.sec_integration_kind} />
                <Row label="Vendor documentation" value={yesNo(d.sec_vendor_docs)} />
                {d.sec_vendor_docs === "yes" && <Row label="Documentation link" value={d.sec_vendor_docs_link} />}
                <Row label="Integration detail" value={d.sec_integration_detail} pre />
              </>
            )}
          </>
        )}
        {pr.category === "NON-IT" && (
          <>
            <Row label="Non-IT category" value={d.nit_category} />
            <Row label="Details" value={d.nit_description} pre />
            <Row label="Additional specs / links" value={d.nit_specs} pre />
          </>
        )}
        {pr.category === "EVENTS" && (
          <Row label="Status" value="Marketing & Events requirement form is under development — Procurement will follow up directly." pre />
        )}
      </Section>

      <Section title="Vendor & budget">
        <Row label="Supplier" value={d.supplier_name} />
        <Row label="Website" value={d.supplier_website} />
        <Row label="Contact" value={d.supplier_contact} />
        <Row label="Contact email" value={d.supplier_email} />
        <Row label="Contact number" value={d.supplier_phone} />
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
        <Row label="Budget category" value={d.budget_category} />
        <Row label="Product" value={d.budget_product} />
        <Row label="Region" value={d.budget_region} />
        <Row label="Engagement code" value={d.engagement_code} />
        <Row label="Notes" value={d.notes} pre />
      </Section>

      {pr.status === "rejected" && pr.rejection_reason && (
        <Alert severity="error">
          <Typography variant="caption" sx={{ fontWeight: 600, textTransform: "uppercase", letterSpacing: "0.05em", display: "block" }}>
            Rejection reason
          </Typography>
          <Typography variant="body2" sx={{ whiteSpace: "pre-wrap" }}>
            {pr.rejection_reason}
          </Typography>
        </Alert>
      )}
    </Stack>
  );
}

// Section is an expandable group of read-only rows.
function Section({ title, defaultOpen, children }: { title: string; defaultOpen?: boolean; children: React.ReactNode }) {
  return (
    <Box component="details" open={defaultOpen} sx={{ border: 1, borderColor: "divider", borderRadius: 2 }}>
      <Box
        component="summary"
        sx={{
          cursor: "pointer",
          userSelect: "none",
          px: 2,
          py: 1.25,
          fontWeight: 500,
        }}
      >
        {title}
      </Box>
      <Stack component="dl" spacing={1} sx={{ borderTop: 1, borderColor: "divider", px: 2, py: 1.5, m: 0 }}>
        {children}
      </Stack>
    </Box>
  );
}

// Row renders one label/value pair, or nothing when the value is empty.
function Row({ label, value, pre, hint }: { label: string; value?: string; pre?: boolean; hint?: string }) {
  if (!value) return null;
  return (
    <Box sx={{ display: { sm: "flex" }, gap: 2 }}>
      <Typography component="dt" variant="body2" color="text.secondary" sx={{ width: 208, flexShrink: 0 }}>
        {label}
      </Typography>
      <Typography component="dd" variant="body2" sx={{ m: 0, whiteSpace: pre ? "pre-wrap" : "normal" }}>
        {value}
        {hint && (
          <Box component="span" sx={{ display: "block", mt: 0.25, fontStyle: "italic", fontSize: 12, color: "text.secondary" }}>
            {hint}
          </Box>
        )}
      </Typography>
    </Box>
  );
}

