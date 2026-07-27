import { useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
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
      business_unit_id: a.business_unit_id,
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

  if (isLoading)
    return (
      <Box sx={{ maxWidth: 800, mx: "auto", p: { xs: 2, md: 4 } }}>
        <CircularProgress size={24} />
      </Box>
    );
  if (error || !inv)
    return (
      <Box sx={{ maxWidth: 800, mx: "auto", p: { xs: 2, md: 4 } }}>
        <Alert severity="error">Failed to load invoice.</Alert>
      </Box>
    );

  const isReceived = inv.status === "received";
  const setStatus = (s: InvoiceStatus) => statusMutation.mutate(s);

  return (
    <Box sx={{ maxWidth: 800, mx: "auto", p: { xs: 2, md: 4 } }}>
      <Stack direction="row" spacing={1} alignItems="center" sx={{ mb: 2 }}>
        <MuiLink component={Link} to="/invoices" variant="body2">
          Invoices
        </MuiLink>
        <Typography variant="body2" color="text.secondary">
          /
        </Typography>
        <Typography variant="body2" color="text.secondary">
          {invRef(inv.id)}
        </Typography>
      </Stack>

      <ChainStepper prId={inv.purchase_request_id} current={{ kind: "invoice", id: inv.id }} />

      <Stack
        direction="row"
        justifyContent="space-between"
        alignItems="flex-start"
        gap={2}
        sx={{ mb: 2 }}
      >
        <Box>
          <Typography variant="h5" sx={{ fontWeight: 600 }}>
            {inv.vendor_invoice_no || invRef(inv.id)}
          </Typography>
          <Stack direction="row" spacing={1} alignItems="center" sx={{ mt: 1 }}>
            <EntityStatusBadge status={inv.status} />
            <Typography variant="body2" color="text.secondary">
              {inv.vendor?.name ?? `Vendor #${inv.vendor_id}`} · {formatMoney(inv.total_amount, inv.currency)}
            </Typography>
          </Stack>
        </Box>
        {!editing && (
          <Stack direction="row" spacing={1} flexWrap="wrap" justifyContent="flex-end">
            {isReceived && (
              <Button
                variant="outlined"
                color="inherit"
                onClick={() => {
                  setDraft(toInput(inv));
                  setEditing(true);
                }}
              >
                Edit
              </Button>
            )}
            {/* Status transitions */}
            {inv.status === "received" && (
              <Button
                variant="contained"
                onClick={() => setStatus("approved")}
                disabled={statusMutation.isPending}
              >
                Approve
              </Button>
            )}
            {inv.status === "approved" && (
              <>
                <Button
                  variant="outlined"
                  color="inherit"
                  onClick={() => setStatus("received")}
                  disabled={statusMutation.isPending}
                >
                  Revert to received
                </Button>
                <Button
                  variant="contained"
                  color="success"
                  onClick={() => setStatus("paid")}
                  disabled={statusMutation.isPending}
                >
                  Mark as paid
                </Button>
              </>
            )}
            {inv.status === "paid" && (
              <Button
                variant="outlined"
                color="inherit"
                onClick={() => setStatus("approved")}
                disabled={statusMutation.isPending}
              >
                Revert to approved
              </Button>
            )}
            {isReceived && (
              <Button
                variant="outlined"
                color="error"
                onClick={() => {
                  if (confirm(`Delete ${invRef(inv.id)}? This cannot be undone.`)) deleteInvoiceMutation.mutate();
                }}
                disabled={deleteInvoiceMutation.isPending}
              >
                Delete
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
              <InvoiceFields value={draft} onChange={setDraft} />
              <Stack direction="row" spacing={1} sx={{ mt: 3 }}>
                <Button
                  variant="contained"
                  onClick={() => saveMutation.mutate()}
                  disabled={
                    saveMutation.isPending ||
                    !allocationsValid(draft.allocation_mode, draft.cost_allocations, invoiceEffectiveTotal(draft))
                  }
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
              <Field label="Invoice date">{inv.invoice_date}</Field>
              <Field label="Due date">{inv.due_date ?? "—"}</Field>
              <Field label="Total">
                {formatMoney(inv.total_amount, inv.currency)}
                {inv.entered_total != null && inv.items.length > 0 &&
                  Math.abs(inv.entered_total - invoiceItemsTotal(inv.items)) > 0.01 && (
                    <Box component="span" sx={{ ml: 1, fontSize: 12, color: "text.secondary" }}>
                      (entered; line items total {formatMoney(invoiceItemsTotal(inv.items), inv.currency)})
                    </Box>
                  )}
              </Field>
              <Field label="Line items">
                {inv.items.length === 0 ? (
                  "—"
                ) : (
                  <Box component="ul" sx={{ listStyle: "disc", pl: 2.5, m: 0 }}>
                    {inv.items.map((it) => (
                      <li key={it.id}>
                        {it.description} —{" "}
                        <Box component="span" sx={{ color: "text.secondary" }}>
                          qty {it.quantity} × {formatMoney(it.unit_price, inv.currency)}
                        </Box>
                      </li>
                    ))}
                  </Box>
                )}
              </Field>
              <Field label={`Budget units (by ${inv.allocation_mode})`}>
                {inv.cost_allocations.length === 0 ? (
                  "—"
                ) : (
                  <Box component="ul" sx={{ listStyle: "disc", pl: 2.5, m: 0 }}>
                    {inv.cost_allocations.map((a) => (
                      <li key={a.id ?? a.business_unit_id}>
                        {a.business_unit ? a.business_unit.name : `#${a.business_unit_id}`}{" "}
                        <Box component="span" sx={{ color: "text.secondary" }}>
                          {inv.allocation_mode === "percentage"
                            ? `${a.value}% (${formatMoney(a.amount ?? 0, inv.currency)})`
                            : formatMoney(a.amount ?? a.value, inv.currency)}
                        </Box>
                      </li>
                    ))}
                  </Box>
                )}
              </Field>
              {inv.approver && (
                <Field label="Approved by">
                  {inv.approver.name || inv.approver.email}
                </Field>
              )}
              {inv.paid_date && <Field label="Paid on">{inv.paid_date}</Field>}
              <Field label="Note">
                <Box component="span" sx={{ whiteSpace: "pre-wrap" }}>
                  {inv.note || "—"}
                </Box>
              </Field>
            </Stack>
          )}
        </CardContent>
      </Card>

      <DirectParentCard prId={inv.purchase_request_id} current={{ kind: "invoice", id: inv.id }} />

      <Box sx={{ mt: 3 }}>
        <DocumentList
          documents={inv.documents ?? []}
          canEdit
          title="Invoice documents"
          onUpload={(f) => uploadMutation.mutate(f)}
          onDelete={(docId) => deleteDocMutation.mutate(docId)}
          onDownload={(doc: Document) => downloadInvoiceDocument(inv.id, doc)}
          onError={setActionError}
        />
      </Box>

      <RelatedDocuments prId={inv.purchase_request_id} current={{ kind: "invoice", id: inv.id }} />
    </Box>
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
