import { useState } from "react";
import { useMutation } from "@tanstack/react-query";
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Stack,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { FileText, Upload } from "@wso2/oxygen-ui-icons-react";
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
    <Stack spacing={2.5}>
      <DraftContractsSection contract={contract} canEdit={canEdit} invalidate={invalidate} />
      <SignedContractSection contract={contract} canEdit={canEdit} invalidate={invalidate} />
    </Stack>
  );
}

function SectionHeading({ children }: { children: React.ReactNode }) {
  return (
    <Typography
      variant="caption"
      sx={{
        fontWeight: 600,
        textTransform: "uppercase",
        letterSpacing: "0.05em",
        color: "text.secondary",
      }}
    >
      {children}
    </Typography>
  );
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
    <Card variant="outlined">
      <CardContent sx={{ p: 1.5, "&:last-child": { pb: 1.5 } }}>
        <Box sx={{ display: "flex", alignItems: "flex-start", justifyContent: "space-between", gap: 1.5 }}>
          <Button
            variant="text"
            startIcon={<FileText size={16} />}
            onClick={() => downloadContractDocument(contractId, doc)}
            sx={{ textTransform: "none", justifyContent: "flex-start" }}
          >
            {doc.filename}
          </Button>
          {canEdit && (
            <Stack direction="row" spacing={1} sx={{ flexShrink: 0 }}>
              <Button
                variant="text"
                size="small"
                onClick={() => {
                  setNotes(doc.notes);
                  setError(null);
                  setEditing((v) => !v);
                }}
              >
                {editing ? "Close" : doc.notes ? "Edit notes" : "Add notes"}
              </Button>
              <Button
                variant="text"
                size="small"
                color="error"
                disabled={removing}
                onClick={onRemove}
              >
                {removeLabel}
              </Button>
            </Stack>
          )}
        </Box>

        {editing ? (
          <Stack spacing={1} sx={{ mt: 1 }}>
            <TextField
              size="small"
              fullWidth
              multiline
              minRows={2}
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
              placeholder="Notes for this document"
            />
            {error && <Alert severity="error">{error}</Alert>}
            <Stack direction="row" spacing={1}>
              <Button variant="contained" disabled={saveNotes.isPending} onClick={() => saveNotes.mutate()}>
                {saveNotes.isPending ? "Saving…" : "Save notes"}
              </Button>
              <Button
                variant="outlined"
                color="inherit"
                onClick={() => {
                  setEditing(false);
                  setNotes(doc.notes);
                  setError(null);
                }}
              >
                Cancel
              </Button>
            </Stack>
          </Stack>
        ) : (
          doc.notes && (
            <Typography variant="body2" sx={{ mt: 0.75, whiteSpace: "pre-wrap", color: "text.secondary" }}>
              {doc.notes}
            </Typography>
          )
        )}
      </CardContent>
    </Card>
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
    <Box
      sx={{
        p: 1.5,
        borderRadius: 2,
        border: "1px dashed",
        borderColor: "divider",
        bgcolor: "action.hover",
      }}
    >
      <Typography variant="body2" sx={{ mb: 1, fontWeight: 500 }}>
        {label}
      </Typography>
      <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
        <Button variant="outlined" color="inherit" component="label" startIcon={<Upload size={16} />}>
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
        placeholder="Notes (optional)"
      />
      {error && (
        <Alert severity="error" sx={{ mt: 1 }}>
          {error}
        </Alert>
      )}
      <Button
        variant="contained"
        sx={{ mt: 1 }}
        disabled={mutation.isPending || !file}
        onClick={() => mutation.mutate()}
      >
        {mutation.isPending ? "Uploading…" : submitLabel}
      </Button>
    </Box>
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
    <Stack spacing={1}>
      <SectionHeading>Draft contracts</SectionHeading>
      {drafts.length === 0 && !canEdit && (
        <Typography variant="body2" sx={{ color: "text.disabled" }}>
          No draft contracts.
        </Typography>
      )}
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
    </Stack>
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
    <Stack spacing={1}>
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
        <Typography variant="body2" sx={{ color: "text.disabled" }}>
          Not signed yet.
        </Typography>
      )}
    </Stack>
  );
}
