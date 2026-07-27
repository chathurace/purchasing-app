import { useMemo, useRef, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  CircularProgress,
  Divider,
  Link as MuiLink,
  Stack,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import {
  ChevronDown,
  ChevronRight,
  FileText,
  Paperclip,
  Plus,
} from "@wso2/oxygen-ui-icons-react";
import { usePurchaseRequest } from "../hooks/usePurchaseRequests";
import { useBusinessUnitLookup } from "../hooks/useBusinessUnits";
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
  PurchaseRequest,
  PurchaseRequestInput,
  Quotation,
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

  if (isLoading)
    return (
      <Box sx={{ maxWidth: 900, mx: "auto", p: { xs: 2, md: 4 } }}>
        <CircularProgress size={24} />
      </Box>
    );
  if (error || !pr)
    return (
      <Box sx={{ maxWidth: 900, mx: "auto", p: { xs: 2, md: 4 } }}>
        <Alert severity="error">Failed to load request.</Alert>
      </Box>
    );

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
    <Box sx={{ maxWidth: 900, mx: "auto", p: { xs: 2, md: 4 } }}>
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

      {/* Documents */}
      <Card variant="outlined" sx={{ mt: 3 }}>
        <CardContent>
          <Stack direction="row" justifyContent="space-between" alignItems="center" sx={{ mb: 1.5 }}>
            <Typography variant="subtitle1" sx={{ fontWeight: 600 }}>
              Documents
            </Typography>
            {canEdit && (
              <Button variant="text" component="label" startIcon={<Plus size={16} />}>
                Add document
                <input
                  ref={fileRef}
                  type="file"
                  accept=".pdf,.docx"
                  hidden
                  onChange={(e) => onPickFile(e.target.files?.[0] ?? null)}
                />
              </Button>
            )}
          </Stack>
          {pr.documents.length === 0 ? (
            <Typography variant="body2" color="text.secondary">
              No documents attached.
            </Typography>
          ) : (
            <Stack divider={<Divider />}>
              {pr.documents.map((doc) => (
                <DocumentRow
                  key={doc.id}
                  prId={pr.id}
                  doc={doc}
                  canEdit={canEdit}
                  onDelete={() => deleteMutation.mutate(doc.id)}
                />
              ))}
            </Stack>
          )}
        </CardContent>
      </Card>

      <TeamLeadApprovalCard pr={pr} me={me} />

      <AssignmentCard pr={pr} me={me} />

      {procurement && pr.my_can_work && <ProcurementSection pr={pr} />}

      <RecommendationSection pr={pr} />

      <RelatedDocuments prId={pr.id} current={{ kind: "pr", id: pr.id }} />
    </Box>
  );
}

// QuotationRow renders a single quotation on the PR page as an expandable row.
// Collapsed it shows the reference, vendor, notes/total and status; expanded it
// loads the full quotation and shows basic info plus a link to download the
// quotation PDF (and any other documents).
function QuotationRow({ q }: { q: Quotation }) {
  const [open, setOpen] = useState(false);
  return (
    <Box component="li" sx={{ py: 1, listStyle: "none" }}>
      <Stack direction="row" justifyContent="space-between" alignItems="flex-start" gap={1.5}>
        <Box
          component="button"
          type="button"
          onClick={() => setOpen((v) => !v)}
          aria-expanded={open}
          sx={{
            display: "flex",
            minWidth: 0,
            alignItems: "flex-start",
            gap: 1,
            textAlign: "left",
            border: 0,
            background: "none",
            p: 0,
            cursor: "pointer",
            color: "inherit",
          }}
        >
          <Box component="span" sx={{ mt: 0.25, color: "text.secondary", flexShrink: 0 }}>
            {open ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
          </Box>
          <Box sx={{ minWidth: 0 }}>
            <Typography variant="body2" color="primary.main" sx={{ fontWeight: 600 }}>
              {quoRef(q.id)} — {q.vendor?.name ?? `Vendor #${q.vendor_id}`}
            </Typography>
            {q.notes && (
              <Typography variant="body2" color="text.secondary" noWrap>
                {q.notes}
              </Typography>
            )}
            {q.total_amount > 0 && (
              <Typography variant="caption" color="text.secondary" sx={{ display: "block" }}>
                {formatMoney(q.total_amount, q.currency)}
              </Typography>
            )}
          </Box>
        </Box>
        <EntityStatusBadge status={q.status} />
      </Stack>
      {open && <QuotationExpanded quotationId={q.id} />}
    </Box>
  );
}

// QuotationExpanded lazily loads the full quotation (the PR-page list only
// carries summaries) to show its basic info, the primary quotation PDF and any
// other attached documents.
function QuotationExpanded({ quotationId }: { quotationId: number }) {
  const { data: q, isLoading } = useQuotation(quotationId);
  if (isLoading || !q) {
    return (
      <Typography variant="caption" color="text.secondary" sx={{ ml: 3, mt: 1, display: "block" }}>
        Loading…
      </Typography>
    );
  }
  const others = q.documents ?? [];
  return (
    <Box
      sx={{
        ml: 3,
        mt: 1,
        p: 1.5,
        border: 1,
        borderColor: "divider",
        borderRadius: 2,
        bgcolor: "background.default",
      }}
    >
      <Stack spacing={1.5}>
        <Box
          sx={{
            display: "grid",
            gridTemplateColumns: "1fr 1fr",
            columnGap: 2,
            rowGap: 0.5,
          }}
        >
          <Box>
            <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 600 }}>
              Total
            </Typography>
            <Typography variant="body2">{formatMoney(q.total_amount, q.currency)}</Typography>
          </Box>
          <Box>
            <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 600 }}>
              Valid until
            </Typography>
            <Typography variant="body2">{q.valid_until ?? "—"}</Typography>
          </Box>
        </Box>

        {q.notes && (
          <Typography variant="body2" color="text.secondary" sx={{ whiteSpace: "pre-wrap" }}>
            {q.notes}
          </Typography>
        )}

        {q.items.length > 0 && (
          <Box component="ul" sx={{ listStyle: "disc", pl: 2, m: 0, color: "text.secondary" }}>
            {q.items.map((it) => (
              <Typography component="li" variant="body2" key={it.id}>
                {it.description} — qty {it.quantity} × {formatMoney(it.unit_price, q.currency)}
              </Typography>
            ))}
          </Box>
        )}

        <Box>
          <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 600, display: "block" }}>
            Quotation PDF
          </Typography>
          {q.quotation_document ? (
            <MuiLink
              component="button"
              type="button"
              variant="body2"
              onClick={() => downloadQuotationDocument(q.id, q.quotation_document!)}
              sx={{ display: "inline-flex", alignItems: "center", gap: 0.5 }}
            >
              <FileText size={14} />
              {q.quotation_document.filename}
            </MuiLink>
          ) : (
            <Typography variant="body2" color="text.secondary">
              None attached.
            </Typography>
          )}
        </Box>

        {others.length > 0 && (
          <Box>
            <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 600, display: "block" }}>
              Other documents
            </Typography>
            <Stack spacing={0.25}>
              {others.map((doc) => (
                <MuiLink
                  key={doc.id}
                  component="button"
                  type="button"
                  variant="body2"
                  onClick={() => downloadQuotationDocument(q.id, doc)}
                  sx={{ display: "inline-flex", alignItems: "center", gap: 0.5, alignSelf: "flex-start" }}
                >
                  <Paperclip size={14} />
                  {doc.filename}
                </MuiLink>
              ))}
            </Stack>
          </Box>
        )}

        <MuiLink component={Link} to={`/quotations/${q.id}`} variant="body2" sx={{ alignSelf: "flex-start" }}>
          Open quotation →
        </MuiLink>
      </Stack>
    </Box>
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
          {!closed && !showReject && (
            <Button variant="text" color="error" size="small" onClick={() => setShowReject(true)}>
              Reject request
            </Button>
          )}
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
          <Stack component="ul" divider={<Divider />} sx={{ listStyle: "none", p: 0, m: 0 }}>
            {quotations.map((q) => (
              <QuotationRow key={q.id} q={q} />
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
                    Record a vendor's quote against this request and attach its PDF.
                  </Typography>
                </Box>
              </Stack>
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
                <Typography variant="body2" sx={{ fontWeight: 500, mb: 0.5 }}>
                  Quotation PDF (optional)
                </Typography>
                <Button variant="outlined" color="inherit" size="small" component="label">
                  {file ? file.name : "Choose PDF"}
                  <input
                    ref={fileRef}
                    type="file"
                    accept=".pdf"
                    hidden
                    onChange={(e) => onPickFile(e.target.files?.[0] ?? null)}
                  />
                </Button>
              </Box>
              <Button
                variant="contained"
                fullWidth
                disabled={createMutation.isPending || !vendorId}
                onClick={() => createMutation.mutate()}
              >
                {createMutation.isPending ? "Adding…" : "Add quotation"}
              </Button>
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
    <Stack direction="row" justifyContent="space-between" alignItems="center" sx={{ py: 1 }}>
      <MuiLink
        component="button"
        type="button"
        variant="body2"
        onClick={() => downloadDocument(prId, doc)}
        sx={{ textAlign: "left" }}
      >
        {doc.filename}
      </MuiLink>
      <Stack direction="row" spacing={1.5} alignItems="center">
        <Typography variant="caption" color="text.secondary">
          {(doc.size_bytes / 1024).toFixed(0)} KB
        </Typography>
        {canEdit && (
          <Button variant="text" color="error" size="small" onClick={onDelete}>
            Remove
          </Button>
        )}
      </Stack>
    </Stack>
  );
}
