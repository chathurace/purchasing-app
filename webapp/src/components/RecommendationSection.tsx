import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  alpha,
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Checkbox,
  Chip,
  FormControlLabel,
  IconButton,
  Link as MuiLink,
  MenuItem,
  Stack,
  Switch,
  TextField,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import type { Theme } from "@wso2/oxygen-ui";
import {
  Banknote,
  FileText,
  MessageCircleQuestion,
  Paperclip,
  Pencil,
  Plus,
  Scale,
  ShieldCheck,
  Trash2,
  X,
} from "@wso2/oxygen-ui-icons-react";
import { useProcurementAccess } from "../hooks/useProcurementAccess";
import { useBusinessUnitApprovers } from "../hooks/useBusinessUnits";
import { useDirectory } from "../hooks/useDirectory";
import { useQuotationsForPR } from "../hooks/useQuotations";
import { useConfigLookup, optionsFor } from "../hooks/useConfigOptions";
import { ApiError } from "../api/client";
import {
  addBudgetStep,
  addRecComment,
  createRecommendation,
  createRecommendationContract,
  deleteBudgetStep,
  deleteRecommendation,
  deleteRecommendationContract,
  deleteRecommendationRFI,
  deleteRecRFIDocument,
  downloadRecCommentDocument,
  downloadRecRFIDocument,
  remindBudgetApprovers,
  remindBudgetStep,
  remindRecAssignee,
  removeRecApproval,
  requestRecApproval,
  setBudgetStepDecision,
  setRecApproval,
  setRecAssignee,
  setRecommendationRFI,
  updateBudgetStep,
  updateRecommendation,
  uploadRecCommentDocument,
  uploadRecRFIDocument,
} from "../api/recommendations";
import { listTeams } from "../api/teams";
import { setBudgetApprover } from "../api/purchaseRequests";
import { uploadContractDocument } from "../api/contracts";
import { ContractContent } from "./ContractContent";
import { EmailAutocomplete } from "./EmailAutocomplete";
import { CurrencyInput } from "./CurrencyInput";
import { ConfirmDialog } from "./ConfirmDialog";
import { EntityStatusBadge } from "./EntityStatusBadge";
import { conRef, REC_APPROVAL_LABELS, REC_APPROVAL_TYPES } from "../types/api";
import type {
  BudgetStep,
  Document,
  PurchaseRequest,
  RecApproval,
  RecApprovalType,
  RecComment,
  Recommendation,
  UserSummary,
} from "../types/api";

interface QuotedVendor {
  id: number;
  name: string;
  registered: boolean;
}

// FieldLabel is the small caption above a form control (was a Tailwind label).
function FieldLabel({ children }: { children: React.ReactNode }) {
  return (
    <Typography variant="body2" sx={{ mb: 0.5, fontWeight: 500 }}>
      {children}
    </Typography>
  );
}

// InlineError renders a small inline error message under a control.
function InlineError({ children }: { children: React.ReactNode }) {
  if (!children) return null;
  return (
    <Typography variant="body2" sx={{ color: "error.main" }}>
      {children}
    </Typography>
  );
}

// RecommendationSection renders the PR's procurement recommendation: procurement adds
// or edits it (once at least one quotation exists), and the named actors
// (legal/security/budget owner) toggle approval and comment on their card.
export function RecommendationSection({ pr }: { pr: PurchaseRequest }) {
  // Authoring the recommendation requires procurement access AND being on the PR
  // (assignee/collaborator/admin) — the same gate the backend enforces. Approver
  // card actions (comment/approve/assign) are driven by per-card flags, not this.
  const procurement = useProcurementAccess() && !!pr.my_can_work;
  // Quoted vendors feed the create/edit form (procurement only); a non-procurement actor
  // acting on a card never needs them, so skip the procurement-gated fetch for them.
  const { data: quotations } = useQuotationsForPR(pr.id, procurement);
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

  // Nothing to show for a non-procurement viewer until a recommendation exists.
  if (!rec && !procurement) return null;
  // Procurement can only add a recommendation once a quotation is in.
  if (!rec && quotedVendors.length === 0) return null;

  return (
    <Card variant="outlined" sx={{ mt: 3 }}>
      <CardContent>
        <Typography variant="h6" sx={{ mb: 1.5 }}>
          Procurement recommendation
        </Typography>
        {rec ? (
          <RecommendationView pr={pr} rec={rec} procurement={procurement} quotedVendors={quotedVendors} />
        ) : (
          <RecommendationForm pr={pr} quotedVendors={quotedVendors} />
        )}
      </CardContent>
    </Card>
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
  const [estimatedValue, setEstimatedValue] = useState<number>(existing?.estimated_value ?? 0);
  const [currency, setCurrency] = useState(existing?.currency ?? "");
  const [engagementType, setEngagementType] = useState(existing?.engagement_type ?? "");
  const [types, setTypes] = useState<RecApprovalType[]>(
    existing ? existing.approvals.map((a) => a.approval_type) : ["budget"],
  );
  const [error, setError] = useState<string | null>(null);
  const { data: config } = useConfigLookup();
  const lists = config?.lists;

  const invalidate = () => qc.invalidateQueries({ queryKey: ["purchase-requests", pr.id] });

  const mutation = useMutation({
    mutationFn: () => {
      const input = {
        vendor_id: vendorId,
        description: description.trim(),
        estimated_value: estimatedValue,
        currency,
        engagement_type: engagementType,
        required_types: types,
      };
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
    <Stack spacing={2}>
      {editing && (
        <Alert severity="warning" variant="outlined">
          Editing the recommendation resets all approvals back to pending.
        </Alert>
      )}
      <Box>
        <FieldLabel>Vendor</FieldLabel>
        <TextField
          select
          size="small"
          fullWidth
          value={vendorId || ""}
          onChange={(e) => setVendorId(Number(e.target.value))}
        >
          <MenuItem value="">Select a quoted vendor…</MenuItem>
          {quotedVendors.map((v) => (
            <MenuItem key={v.id} value={v.id}>
              {v.name}
              {!v.registered ? " (unregistered)" : ""}
            </MenuItem>
          ))}
        </TextField>
      </Box>
      <Box>
        <FieldLabel>Description</FieldLabel>
        <TextField
          size="small"
          fullWidth
          multiline
          minRows={2}
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder="Why this vendor (optional)"
        />
      </Box>
      <Box sx={{ display: "grid", gap: 2, gridTemplateColumns: { sm: "repeat(3, 1fr)" } }}>
        <Box>
          <FieldLabel>Estimated value</FieldLabel>
          <TextField
            type="number"
            size="small"
            fullWidth
            inputProps={{ min: 0, step: "0.01" }}
            value={estimatedValue || ""}
            onChange={(e) => setEstimatedValue(Number(e.target.value))}
            placeholder="0.00"
          />
        </Box>
        <Box>
          <FieldLabel>Currency</FieldLabel>
          <Box sx={{ "& .MuiTextField-root": { width: "100%" } }}>
            <CurrencyInput
              value={currency}
              onChange={setCurrency}
              maxLength={3}
              options={optionsFor(lists, "currency", currency)}
            />
          </Box>
        </Box>
        <Box>
          <FieldLabel>Engagement type</FieldLabel>
          <TextField
            select
            size="small"
            fullWidth
            value={engagementType}
            onChange={(e) => setEngagementType(e.target.value)}
          >
            <MenuItem value="">Select…</MenuItem>
            {optionsFor(lists, "engagement_type", engagementType).map((o) => (
              <MenuItem key={o} value={o}>
                {o}
              </MenuItem>
            ))}
          </TextField>
        </Box>
      </Box>
      <Box>
        <FieldLabel>Approvals required</FieldLabel>
        <Stack direction="row" flexWrap="wrap" sx={{ gap: 2 }}>
          {REC_APPROVAL_TYPES.map((t) => (
            <FormControlLabel
              key={t}
              control={
                <Checkbox size="small" checked={types.includes(t)} onChange={() => toggleType(t)} />
              }
              label={REC_APPROVAL_LABELS[t]}
            />
          ))}
        </Stack>
      </Box>
      {error && <InlineError>{error}</InlineError>}
      <Stack direction="row" spacing={1} sx={{ pt: 0.5 }}>
        <Button
          variant="contained"
          disabled={mutation.isPending || !vendorId || types.length === 0}
          onClick={() => mutation.mutate()}
        >
          {mutation.isPending ? "Saving…" : editing ? "Save changes" : "Save recommendation"}
        </Button>
        {editing && (
          <Button variant="outlined" color="inherit" onClick={() => onDone?.()}>
            Cancel
          </Button>
        )}
      </Stack>
    </Stack>
  );
}

// --- read view with approval cards ---

function RecommendationView({
  pr,
  rec,
  procurement,
  quotedVendors,
}: {
  pr: PurchaseRequest;
  rec: Recommendation;
  procurement: boolean;
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
    <Stack spacing={2}>
      <Box sx={{ display: "flex", alignItems: "flex-start", justifyContent: "space-between", gap: 1.5 }}>
        <Box sx={{ display: "flex", flexDirection: "column", gap: 0.5 }}>
          <Box>
            <Typography component="span" variant="body2" sx={{ fontWeight: 500, color: "text.secondary" }}>
              Vendor:{" "}
            </Typography>
            <Typography component="span" variant="body2" sx={{ fontWeight: 500 }}>
              {rec.vendor?.name ?? `Vendor #${rec.vendor_id}`}
            </Typography>
          </Box>
          {rec.description && (
            <Typography variant="body2" sx={{ whiteSpace: "pre-wrap", color: "text.secondary" }}>
              {rec.description}
            </Typography>
          )}
          {(rec.estimated_value > 0 || rec.currency || rec.engagement_type) && (
            <Box sx={{ display: "flex", flexWrap: "wrap", columnGap: 3, rowGap: 0.25, pt: 0.25, color: "text.secondary" }}>
              {(rec.estimated_value > 0 || rec.currency) && (
                <Typography variant="body2" component="span" sx={{ color: "text.secondary" }}>
                  <Box component="span" sx={{ fontWeight: 500 }}>
                    Estimated value:{" "}
                  </Box>
                  {rec.currency} {rec.estimated_value.toLocaleString()}
                </Typography>
              )}
              {rec.engagement_type && (
                <Typography variant="body2" component="span" sx={{ color: "text.secondary" }}>
                  <Box component="span" sx={{ fontWeight: 500 }}>
                    Engagement:{" "}
                  </Box>
                  {rec.engagement_type}
                </Typography>
              )}
            </Box>
          )}
          <Box sx={{ pt: 0.25 }}>
            <Chip
              size="small"
              variant="outlined"
              color={approved === rec.approvals.length ? "success" : "default"}
              label={`${approved} of ${rec.approvals.length} approvals granted`}
            />
          </Box>
        </Box>
        {procurement && (
          <Stack direction="row" spacing={1} sx={{ flexShrink: 0 }}>
            <Button variant="text" size="small" onClick={() => setEditing(true)}>
              Edit
            </Button>
            <Button
              variant="text"
              size="small"
              color="error"
              disabled={deleteMutation.isPending}
              onClick={() => deleteMutation.mutate()}
            >
              Remove
            </Button>
          </Stack>
        )}
      </Box>

      {error && <InlineError>{error}</InlineError>}

      <Box>
        <Box sx={{ mb: 1, display: "flex", flexWrap: "wrap", alignItems: "center", gap: 1 }}>
          <Typography
            variant="caption"
            sx={{ fontWeight: 600, textTransform: "uppercase", letterSpacing: "0.05em", color: "text.secondary" }}
          >
            Required approvals
          </Typography>
          {procurement && <RequestApprovalButtons prId={pr.id} rec={rec} />}
        </Box>
        <Stack spacing={1.5}>
          {rec.approvals.map((a) => (
            <ApprovalCard
              key={a.approval_type}
              prId={pr.id}
              approval={a}
              businessUnitId={pr.business_unit_id}
              prApproverName={pr.budget_approver_name}
              prApproverEmail={pr.budget_approver_email}
              procurement={procurement}
            />
          ))}
        </Stack>
      </Box>

      <RFICard pr={pr} rec={rec} procurement={procurement} />
      <ContractCard pr={pr} rec={rec} procurement={procurement} />
    </Stack>
  );
}

// --- shared chrome for the recommendation's sub-cards ---
//
// Each sub-card (approval / RFI / contract) is an outlined card with a colored
// left accent and an icon tile, so the individual steps of the procurement
// function read as distinct, primary components.

type Tone = "emerald" | "amber" | "indigo" | "violet" | "sky" | "slate";

// Map the legacy tone names onto MUI semantic palette keys.
const TONE_PALETTE: Record<Exclude<Tone, "slate">, "success" | "warning" | "primary" | "secondary" | "info"> = {
  emerald: "success",
  amber: "warning",
  indigo: "primary",
  violet: "secondary",
  sky: "info",
};

function accentColor(tone: Tone): string {
  return tone === "slate" ? "divider" : `${TONE_PALETTE[tone]}.main`;
}

function tileSx(tone: Tone) {
  if (tone === "slate") {
    return {
      bgcolor: (theme: Theme) => alpha(theme.palette.text.secondary, 0.1),
      color: "text.secondary",
      borderColor: (theme: Theme) => alpha(theme.palette.text.secondary, 0.2),
    } as const;
  }
  const key = TONE_PALETTE[tone];
  return {
    bgcolor: (theme: Theme) => alpha(theme.palette[key].main, 0.12),
    color: `${key}.main`,
    borderColor: (theme: Theme) => alpha(theme.palette[key].main, 0.24),
  } as const;
}

// SubCard is the outlined shell shared by the recommendation's sub-cards.
function SubCard({ tone, children }: { tone: Tone; children: React.ReactNode }) {
  return (
    <Card
      variant="outlined"
      sx={{
        overflow: "hidden",
        borderLeft: 4,
        borderLeftColor: accentColor(tone),
        transition: "box-shadow 0.2s",
        "&:hover": { boxShadow: 1 },
      }}
    >
      {children}
    </Card>
  );
}

function IconTile({ tone, children }: { tone: Tone; children: React.ReactNode }) {
  return (
    <Box
      sx={{
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        flexShrink: 0,
        width: 36,
        height: 36,
        borderRadius: 1.5,
        border: 1,
        ...tileSx(tone),
      }}
    >
      {children}
    </Box>
  );
}

// SubCardBody is the tinted lower section (comments, form fields, attachments).
function SubCardBody({ children }: { children: React.ReactNode }) {
  return (
    <Box sx={{ borderTop: 1, borderColor: "divider", bgcolor: "action.hover", px: 2, py: 1.5 }}>
      {children}
    </Box>
  );
}

// SubCardHead is the top row of a sub-card (icon tile + title + trailing content).
function SubCardHead({ children }: { children: React.ReactNode }) {
  return (
    <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, px: 2, py: 1.5 }}>{children}</Box>
  );
}

function ApprovalIcon({ type }: { type: string }) {
  if (type === "legal") return <Scale size={18} />;
  if (type === "security") return <ShieldCheck size={18} />;
  return <Banknote size={18} />;
}

// --- optional RFI card: request for information from the vendor (description + PDFs) ---

function DocChip({ filename, onClick }: { filename: string; onClick: () => void }) {
  return (
    <Chip
      size="small"
      variant="outlined"
      color="primary"
      clickable
      onClick={onClick}
      icon={<Paperclip size={14} />}
      label={filename}
    />
  );
}

function RFICard({
  pr,
  rec,
  procurement,
}: {
  pr: PurchaseRequest;
  rec: Recommendation;
  procurement: boolean;
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

  // Nothing to show for a non-procurement viewer when there's no RFI.
  if (!present && !procurement) return null;

  // Procurement editing (or first-time add) form.
  if (procurement && (editing || !present)) {
    return (
      <SubCard tone="sky">
        <SubCardHead>
          <IconTile tone="sky">
            <MessageCircleQuestion size={18} />
          </IconTile>
          <Box sx={{ minWidth: 0 }}>
            <Typography variant="subtitle2">RFI — request for information</Typography>
            <Typography variant="caption" sx={{ color: "text.secondary" }}>
              {present ? "Update the request or its attachments" : "Optional — ask the vendor for more detail"}
            </Typography>
          </Box>
        </SubCardHead>
        <SubCardBody>
          <TextField
            size="small"
            fullWidth
            multiline
            minRows={2}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="What information are you requesting from the vendor?"
          />
          <Box sx={{ mt: 1, display: "flex", alignItems: "center", gap: 1 }}>
            <Button variant="text" component="label" startIcon={<Paperclip size={16} />}>
              {file ? "Change PDF" : "Attach PDF"}
              <input
                type="file"
                accept="application/pdf,.pdf"
                hidden
                onChange={(e) => setFile(e.target.files?.[0] ?? null)}
              />
            </Button>
            {file && (
              <Typography variant="caption" sx={{ color: "text.secondary" }}>
                {file.name}
              </Typography>
            )}
          </Box>
          {error && (
            <Box sx={{ mt: 1 }}>
              <InlineError>{error}</InlineError>
            </Box>
          )}
          <Stack direction="row" spacing={1} sx={{ mt: 1.5 }}>
            <Button
              variant="contained"
              disabled={saveMutation.isPending || (!present && !file && description.trim() === "")}
              onClick={() => saveMutation.mutate()}
            >
              {saveMutation.isPending ? "Saving…" : present ? "Save RFI" : "Add RFI"}
            </Button>
            {present && (
              <Button
                variant="outlined"
                color="inherit"
                onClick={() => {
                  setEditing(false);
                  setDescription(rec.rfi_description);
                  setFile(null);
                  setError(null);
                }}
              >
                Cancel
              </Button>
            )}
          </Stack>
        </SubCardBody>
      </SubCard>
    );
  }

  // Read view (RFI present).
  return (
    <SubCard tone="sky">
      <SubCardHead>
        <IconTile tone="sky">
          <MessageCircleQuestion size={18} />
        </IconTile>
        <Box sx={{ minWidth: 0 }}>
          <Typography variant="subtitle2">RFI — request for information</Typography>
          <Typography variant="caption" sx={{ color: "text.secondary" }}>
            Raised with the vendor
          </Typography>
        </Box>
        {procurement && (
          <Stack direction="row" spacing={1} sx={{ ml: "auto", flexShrink: 0 }}>
            <Button variant="text" size="small" onClick={() => setEditing(true)}>
              Edit
            </Button>
            <Button
              variant="text"
              size="small"
              color="error"
              disabled={removeMutation.isPending}
              onClick={() => removeMutation.mutate()}
            >
              Remove
            </Button>
          </Stack>
        )}
      </SubCardHead>
      <SubCardBody>
        {rec.rfi_description && (
          <Typography variant="body2" sx={{ whiteSpace: "pre-wrap", color: "text.primary" }}>
            {rec.rfi_description}
          </Typography>
        )}
        {(rec.rfi_documents?.length ?? 0) > 0 && (
          <Box sx={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 1, mt: rec.rfi_description ? 1 : 0 }}>
            {rec.rfi_documents.map((d) => (
              <Box key={d.id} sx={{ display: "inline-flex", alignItems: "center" }}>
                <DocChip filename={d.filename} onClick={() => downloadRecRFIDocument(pr.id, d)} />
                {procurement && (
                  <Tooltip title="Remove attachment">
                    <span>
                      <IconButton
                        size="small"
                        disabled={removeDocMutation.isPending}
                        onClick={() => removeDocMutation.mutate(d.id)}
                        sx={{ ml: 0.25, color: "text.disabled", "&:hover": { color: "error.main" } }}
                      >
                        <X size={14} />
                      </IconButton>
                    </span>
                  </Tooltip>
                )}
              </Box>
            ))}
          </Box>
        )}
        {procurement && (
          <Button
            variant="text"
            component="label"
            startIcon={<Plus size={16} />}
            sx={{ mt: 1 }}
          >
            Add attachment
            <input
              type="file"
              accept="application/pdf,.pdf"
              hidden
              onChange={(e) => {
                const f = e.target.files?.[0];
                if (f) addDocMutation.mutate(f);
                e.target.value = "";
              }}
            />
          </Button>
        )}
        {error && (
          <Box sx={{ mt: 1 }}>
            <InlineError>{error}</InlineError>
          </Box>
        )}
      </SubCardBody>
    </SubCard>
  );
}

// --- optional contract card: attach a contract PDF + description ---

function ContractCard({
  pr,
  rec,
  procurement,
}: {
  pr: PurchaseRequest;
  rec: Recommendation;
  procurement: boolean;
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
    const canRemove = procurement && contract.status === "draft";
    return (
      <SubCard tone="violet">
        <SubCardHead>
          <IconTile tone="violet">
            <FileText size={18} />
          </IconTile>
          <Box sx={{ minWidth: 0 }}>
            <MuiLink component={Link} to={`/contracts/${contract.id}`} sx={{ fontWeight: 600, fontSize: 14 }}>
              {conRef(contract.id)}
            </MuiLink>
            <Typography variant="caption" sx={{ display: "block", color: "text.secondary" }}>
              Contract
            </Typography>
          </Box>
          <Box sx={{ ml: "auto", display: "flex", alignItems: "center", gap: 1.5, flexShrink: 0 }}>
            <EntityStatusBadge status={contract.status} />
            {canRemove && (
              <Button
                variant="text"
                size="small"
                color="error"
                disabled={removeMutation.isPending}
                onClick={() => removeMutation.mutate()}
              >
                Remove
              </Button>
            )}
          </Box>
        </SubCardHead>
        <SubCardBody>
          {error && (
            <Box sx={{ mb: 1 }}>
              <InlineError>{error}</InlineError>
            </Box>
          )}
          <ContractContent contract={contract} canEdit={procurement} invalidate={invalidate} />
        </SubCardBody>
      </SubCard>
    );
  }

  if (!procurement) return null;

  return (
    <SubCard tone="violet">
      <SubCardHead>
        <IconTile tone="violet">
          <FileText size={18} />
        </IconTile>
        <Box sx={{ minWidth: 0 }}>
          <Typography variant="subtitle2">Contract</Typography>
          <Typography variant="caption" sx={{ color: "text.secondary" }}>
            Optional — attach a draft contract PDF to begin
          </Typography>
        </Box>
      </SubCardHead>
      <SubCardBody>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
          <Button variant="text" component="label" startIcon={<Paperclip size={16} />}>
            {file ? "Change PDF" : "Choose PDF"}
            <input
              type="file"
              accept="application/pdf,.pdf"
              hidden
              onChange={(e) => setFile(e.target.files?.[0] ?? null)}
            />
          </Button>
          {file && (
            <Typography variant="caption" sx={{ color: "text.secondary" }}>
              {file.name}
            </Typography>
          )}
        </Box>
        <TextField
          size="small"
          fullWidth
          multiline
          minRows={2}
          sx={{ mt: 1 }}
          value={notes}
          onChange={(e) => setNotes(e.target.value)}
          placeholder="Notes for this draft (optional)"
        />
        {error && (
          <Box sx={{ mt: 1 }}>
            <InlineError>{error}</InlineError>
          </Box>
        )}
        <Button
          variant="contained"
          sx={{ mt: 1.5 }}
          disabled={createMutation.isPending || !file}
          onClick={() => createMutation.mutate()}
        >
          {createMutation.isPending ? "Adding…" : "Add draft contract"}
        </Button>
      </SubCardBody>
    </SubCard>
  );
}

// --- one approval card: toggle + comment thread ---

function reviewerLabel(name?: string | null, email?: string | null, id?: number): string {
  return email || name || (id != null ? `#${id}` : "");
}

// RequestApprovalButtons offers a "Request X approval" pill for each approval type
// not yet required on the recommendation (procurement only).
function RequestApprovalButtons({ prId, rec }: { prId: number; rec: Recommendation }) {
  const qc = useQueryClient();
  const [error, setError] = useState<string | null>(null);
  const present = new Set(rec.approvals.map((a) => a.approval_type));
  const missing = REC_APPROVAL_TYPES.filter((t) => !present.has(t));
  const request = useMutation({
    mutationFn: (t: RecApprovalType) => requestRecApproval(prId, t),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["purchase-requests", prId] });
      setError(null);
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : "Failed to request approval"),
  });
  if (missing.length === 0) return null;
  return (
    <Box sx={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 1 }}>
      {missing.map((t) => (
        <Button
          key={t}
          size="small"
          variant="outlined"
          startIcon={<Plus size={14} />}
          disabled={request.isPending}
          onClick={() => request.mutate(t)}
        >
          Request {t} approval
        </Button>
      ))}
      {error && (
        <Typography variant="caption" sx={{ color: "error.main" }}>
          {error}
        </Typography>
      )}
    </Box>
  );
}

function ApprovalCard({
  prId,
  approval,
  businessUnitId,
  prApproverName,
  prApproverEmail,
  procurement,
}: {
  prId: number;
  approval: RecApproval;
  businessUnitId: number | null;
  prApproverName?: string;
  prApproverEmail?: string;
  procurement: boolean;
}) {
  const qc = useQueryClient();
  const t = approval.approval_type;
  const [comment, setComment] = useState("");
  const [files, setFiles] = useState<File[]>([]);
  const [open, setOpen] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const invalidate = () => qc.invalidateQueries({ queryKey: ["purchase-requests", prId] });

  const [confirmRemove, setConfirmRemove] = useState(false);
  const removeMutation = useMutation({
    mutationFn: () => removeRecApproval(prId, t),
    onSuccess: () => {
      invalidate();
      setConfirmRemove(false);
      setError(null);
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : "Failed to remove approval"),
  });

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

  // Assignee — legal/security cards only (the budget card has no assignee).
  const assignable = t !== "budget";
  const { data: teams } = useQuery({
    queryKey: ["teams"],
    queryFn: listTeams,
    enabled: assignable && approval.can_assign,
  });
  const members: UserSummary[] = teams?.find((tm) => tm.key === t)?.members ?? [];
  // A staged assignment awaiting the notify decision in the confirm dialog.
  const [pending, setPending] = useState<UserSummary | null>(null);
  const [notify, setNotify] = useState(true);
  const [reminded, setReminded] = useState(false);

  const assignMutation = useMutation({
    mutationFn: ({ id, doNotify }: { id: number | null; doNotify: boolean }) =>
      setRecAssignee(prId, t, id, doNotify),
    onSuccess: () => {
      invalidate();
      setPending(null);
      setError(null);
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : "Failed to update assignee"),
  });

  const remindMutation = useMutation({
    mutationFn: () => remindRecAssignee(prId, t),
    onSuccess: () => {
      setReminded(true);
      setError(null);
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : "Failed to send reminder"),
  });

  // Budget card — the budget approver is one of the PR's business unit approvers.
  // Procurement can change it and (re-)notify the approver.
  const isBudget = t === "budget";
  const { data: businessUnitApprovers } = useBusinessUnitApprovers(isBudget ? businessUnitId : null);
  const approverOptions = businessUnitApprovers ?? [];
  const [notified, setNotified] = useState(false);
  const notifyBudgetMutation = useMutation({
    mutationFn: () => remindBudgetApprovers(prId),
    onSuccess: () => {
      setNotified(true);
      setError(null);
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : "Failed to notify approver"),
  });

  // The PR's named budget approver, editable from this card (procurement) by
  // picking from the business unit's approver list.
  const [editingApprover, setEditingApprover] = useState(false);
  const [approverEmail, setApproverEmail] = useState(prApproverEmail ?? "");
  const saveApprover = useMutation({
    mutationFn: (v: { name: string; email: string }) => setBudgetApprover(prId, v.name, v.email),
    onSuccess: () => {
      invalidate();
      setEditingApprover(false);
      setError(null);
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : "Failed to update approver"),
  });
  const saveSelectedApprover = (email: string) => {
    const u = approverOptions.find((a) => a.email === email);
    saveApprover.mutate({ name: u ? u.name || "" : "", email });
  };
  const namedApproverText =
    prApproverName || prApproverEmail
      ? `${prApproverName || ""}${prApproverEmail ? ` · ${prApproverEmail}` : ""}`
      : "—";

  const onPickAssignee = (value: string) => {
    const currentId = approval.assignee_id ?? null;
    if (value === "") {
      // Unassign — no notification to send.
      if (currentId !== null) assignMutation.mutate({ id: null, doNotify: false });
      return;
    }
    const id = Number(value);
    if (id === currentId) return;
    const member = members.find((m) => m.id === id);
    if (!member) return;
    setNotify(true);
    setPending(member); // opens the notify confirm dialog
  };

  const tone: Tone = approval.approved ? "emerald" : "amber";
  return (
    <SubCard tone={tone}>
      <SubCardHead>
        <IconTile tone={tone}>
          <ApprovalIcon type={t} />
        </IconTile>
        <Box sx={{ minWidth: 0 }}>
          <Typography variant="subtitle2">{REC_APPROVAL_LABELS[t]}</Typography>
          <Typography variant="caption" noWrap sx={{ display: "block", color: "text.secondary" }}>
            {approval.approved && approval.approver
              ? `Approved by ${reviewerLabel(approval.approver.name, approval.approver.email)}`
              : "Awaiting sign-off"}
          </Typography>
        </Box>
        <Box sx={{ ml: "auto", display: "flex", alignItems: "center", gap: 1.5, flexShrink: 0 }}>
          <Chip
            size="small"
            variant="outlined"
            color={approval.approved ? "success" : "warning"}
            label={approval.approved ? "Approved" : "Pending"}
          />
          {approval.can_approve && !isBudget && (
            <Tooltip title={approval.approved ? "Revert to pending" : "Approve"}>
              <span>
                <Switch
                  size="small"
                  color="success"
                  checked={approval.approved}
                  disabled={toggleMutation.isPending}
                  onChange={() => toggleMutation.mutate()}
                  inputProps={{ "aria-label": approval.approved ? "Revert to pending" : "Approve" }}
                />
              </span>
            </Tooltip>
          )}
          {procurement && (
            <Tooltip title={`Remove the ${REC_APPROVAL_LABELS[t]} approval`}>
              <IconButton
                size="small"
                onClick={() => setConfirmRemove(true)}
                sx={{ color: "text.disabled", "&:hover": { color: "error.main" } }}
              >
                <X size={16} />
              </IconButton>
            </Tooltip>
          )}
        </Box>
      </SubCardHead>

      {confirmRemove && (
        <ConfirmDialog
          title={`Remove ${REC_APPROVAL_LABELS[t]} approval?`}
          confirmLabel="Remove"
          danger
          busy={removeMutation.isPending}
          message={
            <p>
              Remove the <span style={{ fontWeight: 600 }}>{REC_APPROVAL_LABELS[t]}</span> approval from
              this recommendation? Its comments and attachments
              {isBudget ? " and every approval step" : ""} will be deleted.
            </p>
          }
          onConfirm={() => removeMutation.mutate()}
          onCancel={() => setConfirmRemove(false)}
        />
      )}

      {/* Budget card — the PR's named approver + the designated approver(s) */}
      {isBudget && (
        <Box sx={{ borderTop: 1, borderColor: "divider", bgcolor: "background.paper", px: 2, py: 1.25 }}>
          {/* Named approver (from the PR's business unit), editable by procurement */}
          <Box sx={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 1 }}>
            <Typography
              variant="caption"
              sx={{ fontWeight: 600, textTransform: "uppercase", letterSpacing: "0.05em", color: "text.secondary" }}
            >
              Approver
            </Typography>
            {editingApprover ? (
              <Box sx={{ flex: 1, display: "flex", flexWrap: "wrap", alignItems: "center", gap: 1 }}>
                <TextField
                  select
                  size="small"
                  sx={{ flex: 1, minWidth: 0 }}
                  value={approverEmail}
                  onChange={(e) => setApproverEmail(e.target.value)}
                >
                  <MenuItem value="">— Select approver —</MenuItem>
                  {approverOptions.map((u) => (
                    <MenuItem key={u.id} value={u.email}>
                      {u.name ? `${u.name} (${u.email})` : u.email}
                    </MenuItem>
                  ))}
                </TextField>
                <Button
                  variant="contained"
                  size="small"
                  onClick={() => saveSelectedApprover(approverEmail.trim())}
                  disabled={saveApprover.isPending || !approverEmail.trim()}
                >
                  Save
                </Button>
                <Button
                  variant="outlined"
                  color="inherit"
                  size="small"
                  onClick={() => {
                    setEditingApprover(false);
                    setApproverEmail(prApproverEmail ?? "");
                  }}
                >
                  Cancel
                </Button>
              </Box>
            ) : (
              <>
                <Typography variant="body2" sx={{ color: "text.primary" }}>
                  {namedApproverText}
                </Typography>
                {procurement && (
                  <Tooltip title="Change the budget approver">
                    <IconButton
                      size="small"
                      onClick={() => {
                        setApproverEmail(prApproverEmail ?? "");
                        setEditingApprover(true);
                      }}
                      sx={{ color: "text.secondary" }}
                    >
                      <Pencil size={14} />
                    </IconButton>
                  </Tooltip>
                )}
                {procurement && (prApproverEmail ?? "").trim() !== "" && !approval.approved && (
                  <Button
                    variant="outlined"
                    color="inherit"
                    size="small"
                    sx={{ ml: "auto" }}
                    onClick={() => notifyBudgetMutation.mutate()}
                    disabled={notifyBudgetMutation.isPending}
                  >
                    {notified ? "Notified ✓" : "Notify approver"}
                  </Button>
                )}
              </>
            )}
          </Box>
        </Box>
      )}

      {/* Serial budget approval chain */}
      {isBudget && (
        <BudgetChain
          prId={prId}
          steps={approval.budget_steps ?? []}
          procurement={procurement}
          baseApproverText={namedApproverText}
        />
      )}

      {/* Assignee (legal/security) */}
      {assignable && (approval.assignee || approval.can_assign) && (
        <Box
          sx={{
            display: "flex",
            flexWrap: "wrap",
            alignItems: "center",
            gap: 1,
            borderTop: 1,
            borderColor: "divider",
            bgcolor: "background.paper",
            px: 2,
            py: 1.25,
          }}
        >
          <Typography
            variant="caption"
            sx={{ fontWeight: 600, textTransform: "uppercase", letterSpacing: "0.05em", color: "text.secondary" }}
          >
            Assignee
          </Typography>
          {approval.can_assign ? (
            <TextField
              select
              size="small"
              value={approval.assignee_id ?? ""}
              disabled={assignMutation.isPending}
              onChange={(e) => onPickAssignee(e.target.value)}
              sx={{ minWidth: 180 }}
            >
              <MenuItem value="">Unassigned</MenuItem>
              {members.map((m) => (
                <MenuItem key={m.id} value={m.id}>
                  {m.email || m.name || `#${m.id}`}
                </MenuItem>
              ))}
              {/* Keep a stale assignee (no longer a team member) visible/selected. */}
              {approval.assignee && !members.some((m) => m.id === approval.assignee_id) && (
                <MenuItem value={approval.assignee_id ?? ""}>
                  {approval.assignee.email || approval.assignee.name}
                </MenuItem>
              )}
            </TextField>
          ) : (
            <Typography variant="body2" sx={{ color: "text.primary" }}>
              {approval.assignee
                ? reviewerLabel(approval.assignee.name, approval.assignee.email)
                : "Unassigned"}
            </Typography>
          )}
          {approval.assignee && approval.can_assign && (
            <Button
              variant="outlined"
              size="small"
              disabled={remindMutation.isPending || assignMutation.isPending}
              onClick={() => {
                setReminded(false);
                remindMutation.mutate();
              }}
            >
              {remindMutation.isPending ? "Sending…" : reminded ? "Reminder sent ✓" : "Send reminder"}
            </Button>
          )}
          {error && (
            <Typography variant="caption" sx={{ color: "error.main" }}>
              {error}
            </Typography>
          )}
        </Box>
      )}

      {pending && (
        <ConfirmDialog
          title="Notify assignee?"
          confirmLabel="Assign"
          busy={assignMutation.isPending}
          message={
            <Stack spacing={1.5}>
              <p>
                Assign the {REC_APPROVAL_LABELS[t]} approval to{" "}
                <span style={{ fontWeight: 600 }}>{reviewerLabel(pending.name, pending.email)}</span>?
              </p>
              <FormControlLabel
                control={
                  <Checkbox
                    size="small"
                    checked={notify}
                    onChange={(e) => setNotify(e.target.checked)}
                  />
                }
                label="Email them a link to this request (CC the team email)"
              />
            </Stack>
          }
          onConfirm={() => assignMutation.mutate({ id: pending.id, doNotify: notify })}
          onCancel={() => setPending(null)}
        />
      )}

      {/* Comments (legal/security card-level; budget comments live on each step) */}
      {!isBudget && (approval.comments.length > 0 || approval.can_comment) && (
        <SubCardBody>
          {approval.comments.length > 0 && (
            <Stack component="ul" spacing={1} sx={{ listStyle: "none", m: 0, p: 0 }}>
              {approval.comments.map((c) => (
                <CommentItem key={c.id} prId={prId} comment={c} />
              ))}
            </Stack>
          )}

          {approval.can_comment &&
            (open ? (
              <Stack spacing={1} sx={{ mt: 1.5 }}>
                <TextField
                  size="small"
                  fullWidth
                  multiline
                  minRows={2}
                  value={comment}
                  onChange={(e) => setComment(e.target.value)}
                  placeholder="Comment"
                />
                <Box>
                  <Button variant="text" component="label" startIcon={<Paperclip size={16} />}>
                    Attach documents
                    <input
                      type="file"
                      multiple
                      hidden
                      onChange={(e) => setFiles(Array.from(e.target.files ?? []))}
                    />
                  </Button>
                  {files.length > 0 && (
                    <Box component="ul" sx={{ mt: 0.5, pl: 2, color: "text.secondary" }}>
                      {files.map((f, i) => (
                        <Typography component="li" variant="caption" key={i}>
                          {f.name}
                        </Typography>
                      ))}
                    </Box>
                  )}
                </Box>
                {error && <InlineError>{error}</InlineError>}
                <Stack direction="row" spacing={1}>
                  <Button
                    variant="contained"
                    size="small"
                    disabled={commentMutation.isPending || (comment.trim() === "" && files.length === 0)}
                    onClick={() => commentMutation.mutate()}
                  >
                    {commentMutation.isPending ? "Saving…" : "Add comment"}
                  </Button>
                  <Button
                    variant="text"
                    color="inherit"
                    size="small"
                    onClick={() => {
                      setOpen(false);
                      setComment("");
                      setFiles([]);
                    }}
                  >
                    Cancel
                  </Button>
                </Stack>
              </Stack>
            ) : (
              <Button
                variant="text"
                size="small"
                startIcon={<Plus size={16} />}
                onClick={() => setOpen(true)}
                sx={{ mt: approval.comments.length > 0 ? 1.5 : 0 }}
              >
                Add comment
              </Button>
            ))}
        </SubCardBody>
      )}
    </SubCard>
  );
}

// --- serial budget approval chain ---

// CommentItem renders a single comment (author, timestamp, body, attachments).
function CommentItem({ prId, comment: c }: { prId: number; comment: RecComment }) {
  return (
    <Card component="li" variant="outlined" sx={{ px: 1.5, py: 1 }}>
      <Box sx={{ display: "flex", alignItems: "baseline", justifyContent: "space-between", gap: 1 }}>
        <Typography variant="body2" sx={{ fontWeight: 500 }}>
          {reviewerLabel(c.author?.name, c.author?.email, c.author_id)}
        </Typography>
        <Typography variant="caption" sx={{ flexShrink: 0, color: "text.disabled" }}>
          {new Date(c.created_at).toLocaleString()}
        </Typography>
      </Box>
      {c.comment && (
        <Typography variant="body2" sx={{ mt: 0.5, whiteSpace: "pre-wrap", color: "text.primary" }}>
          {c.comment}
        </Typography>
      )}
      {c.documents.length > 0 && (
        <Box sx={{ mt: 0.75, display: "flex", flexWrap: "wrap", gap: 1 }}>
          {c.documents.map((d: Document) => (
            <Chip
              key={d.id}
              size="small"
              variant="outlined"
              color="primary"
              clickable
              icon={<Paperclip size={14} />}
              label={d.filename}
              onClick={() => downloadRecCommentDocument(prId, c.id, d)}
            />
          ))}
        </Box>
      )}
    </Card>
  );
}

// CommentThread renders a comment list plus an "add comment" form (text + file
// attachments). onSubmit performs the add + uploads; the parent invalidates.
function CommentThread({
  prId,
  comments,
  canComment,
  onSubmit,
}: {
  prId: number;
  comments: RecComment[];
  canComment: boolean;
  onSubmit: (text: string, files: File[]) => Promise<void>;
}) {
  const [comment, setComment] = useState("");
  const [files, setFiles] = useState<File[]>([]);
  const [open, setOpen] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const mutation = useMutation({
    mutationFn: () => onSubmit(comment.trim(), files),
    onSuccess: () => {
      setComment("");
      setFiles([]);
      setOpen(false);
      setError(null);
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : "Failed to add comment"),
  });
  if (comments.length === 0 && !canComment) return null;
  return (
    <Box>
      {comments.length > 0 && (
        <Stack component="ul" spacing={1} sx={{ listStyle: "none", m: 0, p: 0 }}>
          {comments.map((c) => (
            <CommentItem key={c.id} prId={prId} comment={c} />
          ))}
        </Stack>
      )}
      {canComment &&
        (open ? (
          <Stack spacing={1} sx={{ mt: 1.5 }}>
            <TextField
              size="small"
              fullWidth
              multiline
              minRows={2}
              value={comment}
              onChange={(e) => setComment(e.target.value)}
              placeholder="Comment"
            />
            <Box>
              <Button variant="text" component="label" startIcon={<Paperclip size={16} />}>
                Attach documents
                <input
                  type="file"
                  multiple
                  hidden
                  onChange={(e) => setFiles(Array.from(e.target.files ?? []))}
                />
              </Button>
              {files.length > 0 && (
                <Box component="ul" sx={{ mt: 0.5, pl: 2, color: "text.secondary" }}>
                  {files.map((f, i) => (
                    <Typography component="li" variant="caption" key={i}>
                      {f.name}
                    </Typography>
                  ))}
                </Box>
              )}
            </Box>
            {error && <InlineError>{error}</InlineError>}
            <Stack direction="row" spacing={1}>
              <Button
                variant="contained"
                size="small"
                disabled={mutation.isPending || (comment.trim() === "" && files.length === 0)}
                onClick={() => mutation.mutate()}
              >
                {mutation.isPending ? "Saving…" : "Add comment"}
              </Button>
              <Button
                variant="text"
                color="inherit"
                size="small"
                onClick={() => {
                  setOpen(false);
                  setComment("");
                  setFiles([]);
                }}
              >
                Cancel
              </Button>
            </Stack>
          </Stack>
        ) : (
          <Button
            variant="text"
            size="small"
            startIcon={<Plus size={16} />}
            onClick={() => setOpen(true)}
            sx={{ mt: comments.length > 0 ? 1.5 : 0 }}
          >
            Add comment
          </Button>
        ))}
    </Box>
  );
}

// Per-decision presentation for a budget step.
const STEP_TONE: Record<
  BudgetStep["decision"],
  { color: "success" | "error" | "warning"; accent: string; label: string }
> = {
  approved: { color: "success", accent: "success.main", label: "Approved" },
  rejected: { color: "error", accent: "error.main", label: "Rejected" },
  pending: { color: "warning", accent: "divider", label: "Pending" },
};

// BudgetChain renders the base (default) budget approver plus, when present, the
// ordered "Additional approvals" chain and — for procurement — the control to
// append a new named step.
function BudgetChain({
  prId,
  steps,
  procurement,
  baseApproverText,
}: {
  prId: number;
  steps: BudgetStep[];
  procurement: boolean;
  baseApproverText: string;
}) {
  const qc = useQueryClient();
  const invalidate = () => qc.invalidateQueries({ queryKey: ["purchase-requests", prId] });
  const [adding, setAdding] = useState(false);
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [error, setError] = useState<string | null>(null);
  const { data: directory, isLoading: dirLoading, refresh: refreshDir } = useDirectory(procurement && adding);
  const add = useMutation({
    mutationFn: () => addBudgetStep(prId, name.trim(), email.trim()),
    onSuccess: () => {
      invalidate();
      setAdding(false);
      setName("");
      setEmail("");
      setError(null);
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : "Failed to add step"),
  });
  const ordered = [...steps].sort((a, b) => a.position - b.position);
  const base = ordered.find((s) => s.is_base);
  const additional = ordered.filter((s) => !s.is_base);
  // An additional step is actionable only after every earlier step is approved.
  const stepLocked = (s: BudgetStep) => {
    const prior = ordered.filter((o) => o.position < s.position);
    return s.decision === "pending" && prior.some((o) => o.decision !== "approved");
  };
  return (
    <Stack spacing={1} sx={{ borderTop: 1, borderColor: "divider", bgcolor: "background.paper", px: 2, py: 1.25 }}>
      {base && (
        <Box component="ol" sx={{ listStyle: "none", m: 0, p: 0, display: "flex", flexDirection: "column", gap: 1 }}>
          <BudgetStepCard
            key={base.id}
            prId={prId}
            step={base}
            title="Budget approver"
            approverText={baseApproverText}
            locked={false}
          />
        </Box>
      )}

      {additional.length > 0 && (
        <Stack spacing={1} sx={{ pt: 0.5 }}>
          <Typography
            variant="caption"
            sx={{ fontWeight: 600, textTransform: "uppercase", letterSpacing: "0.05em", color: "text.secondary" }}
          >
            Additional approvals
          </Typography>
          <Box component="ol" sx={{ listStyle: "none", m: 0, p: 0, display: "flex", flexDirection: "column", gap: 1 }}>
            {additional.map((s, i) => (
              <BudgetStepCard
                key={s.id}
                prId={prId}
                step={s}
                title={`Step ${i + 1}`}
                locked={stepLocked(s)}
              />
            ))}
          </Box>
        </Stack>
      )}

      {procurement &&
        (adding ? (
          <Stack
            spacing={1}
            sx={{ borderRadius: 1.5, border: "1px dashed", borderColor: "divider", bgcolor: "action.hover", px: 1.5, py: 1.25 }}
          >
            <Typography variant="caption" sx={{ fontWeight: 500, color: "text.secondary" }}>
              Add an approval step
            </Typography>
            <EmailAutocomplete
              value={email}
              onChange={(picked, u) => {
                setEmail(picked);
                // The approver's display name comes from the directory match (if
                // any); a free-typed email is added name-less (the backend only
                // requires the email).
                setName(u?.name ?? "");
              }}
              directory={directory ?? []}
              refresh={refreshDir}
              loading={dirLoading}
              placeholder="approver@wso2.com"
              ariaLabel="Approver email"
            />
            {error && (
              <Typography variant="caption" sx={{ color: "error.main" }}>
                {error}
              </Typography>
            )}
            <Stack direction="row" spacing={1}>
              <Button
                variant="contained"
                size="small"
                disabled={add.isPending || !email.trim().includes("@")}
                onClick={() => add.mutate()}
              >
                {add.isPending ? "Adding…" : "Add step"}
              </Button>
              <Button
                variant="outlined"
                color="inherit"
                size="small"
                onClick={() => {
                  setAdding(false);
                  setName("");
                  setEmail("");
                  setError(null);
                }}
              >
                Cancel
              </Button>
            </Stack>
          </Stack>
        ) : (
          <Box>
            <Button variant="text" size="small" startIcon={<Plus size={16} />} onClick={() => setAdding(true)}>
              Add another approval step
            </Button>
          </Box>
        ))}
    </Stack>
  );
}

// BudgetStepCard renders one step of the chain: its approver, decision state,
// decision controls (for the step's approver), management controls (procurement),
// and a comment thread.
function BudgetStepCard({
  prId,
  step,
  title,
  approverText,
  locked,
}: {
  prId: number;
  step: BudgetStep;
  title: string;
  // Overrides the approver line (used for the base step to show the designated
  // approver). Defaults to the step's named approver.
  approverText?: string;
  locked: boolean;
}) {
  const qc = useQueryClient();
  const invalidate = () => qc.invalidateQueries({ queryKey: ["purchase-requests", prId] });
  const [error, setError] = useState<string | null>(null);
  const [editing, setEditing] = useState(false);
  const [name, setName] = useState(step.approver_name);
  const [email, setEmail] = useState(step.approver_email);
  const [reminded, setReminded] = useState(false);
  const { data: directory, isLoading: dirLoading, refresh: refreshDir } = useDirectory(editing);
  const tone = STEP_TONE[step.decision];

  const decide = useMutation({
    mutationFn: (d: "approve" | "reject" | "revert") => setBudgetStepDecision(prId, step.id, d),
    onSuccess: invalidate,
    onError: (e) => setError(e instanceof ApiError ? e.message : "Failed to record decision"),
  });
  const save = useMutation({
    mutationFn: () => updateBudgetStep(prId, step.id, name.trim(), email.trim()),
    onSuccess: () => {
      invalidate();
      setEditing(false);
      setError(null);
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : "Failed to update step"),
  });
  const remove = useMutation({
    mutationFn: () => deleteBudgetStep(prId, step.id),
    onSuccess: invalidate,
    onError: (e) => setError(e instanceof ApiError ? e.message : "Failed to remove step"),
  });
  const remind = useMutation({
    mutationFn: () => remindBudgetStep(prId, step.id),
    onSuccess: () => setReminded(true),
    onError: (e) => setError(e instanceof ApiError ? e.message : "Failed to send reminder"),
  });

  const shownApprover =
    approverText ??
    (step.approver_name || step.approver_email
      ? `${step.approver_name || ""}${step.approver_email ? ` · ${step.approver_email}` : ""}`
      : "—");

  return (
    <Card
      component="li"
      variant="outlined"
      sx={{ borderLeft: 4, borderLeftColor: tone.accent, opacity: locked ? 0.6 : 1 }}
    >
      <Box sx={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 1, px: 1.5, py: 1 }}>
        <Box
          sx={{
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            flexShrink: 0,
            width: 24,
            height: 24,
            borderRadius: "50%",
            bgcolor: "action.hover",
            color: "text.secondary",
            fontSize: 12,
            fontWeight: 600,
          }}
        >
          {step.is_base ? "★" : step.position - 1}
        </Box>
        <Box sx={{ minWidth: 0 }}>
          <Typography variant="caption" sx={{ fontWeight: 600, color: "text.secondary" }}>
            {title}
          </Typography>
          {editing ? null : (
            <Typography variant="body2" noWrap sx={{ color: "text.primary" }}>
              {shownApprover}
            </Typography>
          )}
        </Box>
        <Box sx={{ ml: "auto", display: "flex", alignItems: "center", gap: 1, flexShrink: 0 }}>
          <Chip size="small" variant="outlined" color={tone.color} label={tone.label} />
        </Box>
      </Box>

      {/* Edit approver (additional steps, procurement) */}
      {editing && (
        <Box sx={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 1, borderTop: 1, borderColor: "divider", px: 1.5, py: 1 }}>
          <TextField
            size="small"
            sx={{ flex: 1, minWidth: 0 }}
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="Approver name"
          />
          <Box sx={{ flex: 1, minWidth: 0 }}>
            <EmailAutocomplete
              value={email}
              onChange={(picked, u) => {
                setEmail(picked);
                if (u && u.name) setName(u.name);
              }}
              directory={directory ?? []}
              refresh={refreshDir}
              loading={dirLoading}
              placeholder="approver@wso2.com"
              ariaLabel="Approver email"
            />
          </Box>
          <Button
            variant="contained"
            size="small"
            disabled={save.isPending || !email.trim().includes("@")}
            onClick={() => save.mutate()}
          >
            Save
          </Button>
          <Button
            variant="outlined"
            color="inherit"
            size="small"
            onClick={() => {
              setEditing(false);
              setName(step.approver_name);
              setEmail(step.approver_email);
            }}
          >
            Cancel
          </Button>
        </Box>
      )}

      {/* Decision + management controls */}
      <Box sx={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 1, borderTop: 1, borderColor: "divider", px: 1.5, py: 1 }}>
        {step.decision !== "pending" && step.decider && (
          <Typography variant="caption" sx={{ color: "text.secondary" }}>
            {tone.label} by {reviewerLabel(step.decider.name, step.decider.email)}
            {step.decided_at ? ` · ${new Date(step.decided_at).toLocaleDateString()}` : ""}
          </Typography>
        )}
        {locked && step.decision === "pending" && (
          <Typography variant="caption" sx={{ color: "text.disabled" }}>
            Awaiting the previous step
          </Typography>
        )}
        <Box sx={{ ml: "auto", display: "flex", alignItems: "center", gap: 1 }}>
          {step.can_decide && step.decision === "pending" && (
            <>
              <Button
                variant="contained"
                color="success"
                size="small"
                disabled={decide.isPending}
                onClick={() => decide.mutate("approve")}
              >
                Approve
              </Button>
              <Button
                variant="outlined"
                color="error"
                size="small"
                disabled={decide.isPending}
                onClick={() => decide.mutate("reject")}
              >
                Reject
              </Button>
            </>
          )}
          {step.can_decide && step.decision !== "pending" && (
            <Button
              variant="outlined"
              color="inherit"
              size="small"
              disabled={decide.isPending}
              onClick={() => decide.mutate("revert")}
            >
              Reverse decision
            </Button>
          )}
          {step.can_manage && !editing && (
            <>
              <Tooltip title="Edit approver">
                <IconButton
                  size="small"
                  onClick={() => {
                    setName(step.approver_name);
                    setEmail(step.approver_email);
                    setEditing(true);
                  }}
                  sx={{ color: "text.secondary" }}
                >
                  <Pencil size={14} />
                </IconButton>
              </Tooltip>
              <Tooltip title="Remove step">
                <span>
                  <IconButton
                    size="small"
                    disabled={remove.isPending}
                    onClick={() => remove.mutate()}
                    sx={{ color: "text.secondary", "&:hover": { color: "error.main" } }}
                  >
                    <Trash2 size={14} />
                  </IconButton>
                </span>
              </Tooltip>
              {!step.is_base && (
                <Button
                  variant="outlined"
                  size="small"
                  disabled={remind.isPending}
                  onClick={() => {
                    setReminded(false);
                    remind.mutate();
                  }}
                >
                  {remind.isPending ? "Sending…" : reminded ? "Reminded ✓" : "Remind"}
                </Button>
              )}
            </>
          )}
        </Box>
      </Box>

      {error && (
        <Typography variant="caption" sx={{ display: "block", px: 1.5, pb: 1, color: "error.main" }}>
          {error}
        </Typography>
      )}

      {/* Comments on this step */}
      {(step.comments.length > 0 || step.can_decide || step.can_manage) && (
        <Box sx={{ borderTop: 1, borderColor: "divider", bgcolor: "action.hover", px: 1.5, py: 1 }}>
          <CommentThread
            prId={prId}
            comments={step.comments}
            canComment={step.can_decide || step.can_manage}
            onSubmit={async (text, files) => {
              const c = await addRecComment(prId, "budget", text, step.id);
              for (const f of files) await uploadRecCommentDocument(prId, c.id, f);
              invalidate();
            }}
          />
        </Box>
      )}
    </Card>
  );
}
