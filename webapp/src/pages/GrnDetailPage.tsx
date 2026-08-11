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
import { useGRN } from "../hooks/useGrns";
import { GRNFields } from "../components/GRNFields";
import { DocumentList } from "../components/DocumentList";
import { useConfirmAction } from "../components/ConfirmDialog";
import { ChainStepper } from "../components/ChainStepper";
import { DirectParentCard } from "../components/CaseSections";
import { RelatedEntities } from "../components/RelatedEntities";
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
  const [confirmNode, confirmDelete] = useConfirmAction();

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

  if (isLoading)
    return (
      <Box sx={{ maxWidth: 1200, mx: "auto", p: { xs: 2, md: 4 } }}>
        <CircularProgress size={24} />
      </Box>
    );
  if (error || !g)
    return (
      <Box sx={{ maxWidth: 1200, mx: "auto", p: { xs: 2, md: 4 } }}>
        <Alert severity="error">Failed to load GRN.</Alert>
      </Box>
    );

  return (
    <Box sx={{ maxWidth: 1200, mx: "auto", p: { xs: 2, md: 4 } }}>
      <Stack direction="row" spacing={1} alignItems="center" sx={{ mb: 2 }}>
        <MuiLink component={Link} to="/grns" variant="body2">
          GRNs
        </MuiLink>
        <Typography variant="body2" color="text.secondary">
          /
        </Typography>
        <Typography variant="body2" color="text.secondary">
          {grnRef(g.id)}
        </Typography>
      </Stack>

      <ChainStepper prId={g.purchase_request_id} current={{ kind: "grn", id: g.id }} />

      <Stack
        direction="row"
        justifyContent="space-between"
        alignItems="flex-start"
        sx={{ mb: 2 }}
      >
        <Box>
          <Typography variant="h5" sx={{ fontWeight: 600 }}>
            {grnRef(g.id)}
          </Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
            {g.vendor?.name ?? `Vendor #${g.vendor_id}`} · received {g.received_date}
          </Typography>
        </Box>
        {!editing && (
          <Stack direction="row" spacing={1}>
            <Button
              variant="outlined"
              color="inherit"
              onClick={() => {
                setDraft(toInput(g));
                setEditing(true);
              }}
            >
              Edit
            </Button>
            <Button
              variant="outlined"
              color="error"
              onClick={() =>
                confirmDelete({
                  title: "Delete goods received note",
                  message: (
                    <>
                      Delete <strong>{grnRef(g.id)}</strong>? Its line items and attached documents
                      go with it. This cannot be undone.
                    </>
                  ),
                  confirmLabel: "Delete",
                  onConfirm: () => deleteGRNMutation.mutate(),
                })
              }
              disabled={deleteGRNMutation.isPending}
            >
              Delete
            </Button>
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
              <GRNFields value={draft} onChange={setDraft} />
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
              <Field label="Received date">{g.received_date}</Field>
              <Field label="Received by">{g.received_by || "—"}</Field>
              <Field label="Items received">
                {g.items.length === 0 ? (
                  "—"
                ) : (
                  <Box component="ul" sx={{ listStyle: "disc", pl: 2.5, m: 0 }}>
                    {g.items.map((it) => (
                      <li key={it.id}>
                        {it.description} —{" "}
                        <Box component="span" sx={{ color: "text.secondary" }}>
                          qty {it.quantity}
                        </Box>
                      </li>
                    ))}
                  </Box>
                )}
              </Field>
              <Field label="Note">
                <Box component="span" sx={{ whiteSpace: "pre-wrap" }}>
                  {g.note || "—"}
                </Box>
              </Field>
            </Stack>
          )}
        </CardContent>
      </Card>

      <DirectParentCard prId={g.purchase_request_id} current={{ kind: "grn", id: g.id }} />

      <Box sx={{ mt: 3 }}>
        <DocumentList
          documents={g.documents ?? []}
          canEdit
          onUpload={(f) => uploadMutation.mutate(f)}
          onDelete={(docId) => deleteDocMutation.mutate(docId)}
          onDownload={(doc: Document) => downloadGRNDocument(g.id, doc)}
          onError={setActionError}
        />
      </Box>

      <RelatedEntities prId={g.purchase_request_id} current={{ kind: "grn", id: g.id }} />
      {confirmNode}
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
