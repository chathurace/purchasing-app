import { useState } from "react";
import { Button, Dialog, DialogActions, DialogContent, DialogTitle } from "@wso2/oxygen-ui";

interface Props {
  title: string;
  message: React.ReactNode;
  confirmLabel: string;
  // danger styles the confirm button red (for destructive/negative actions).
  danger?: boolean;
  busy?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}

// ConfirmDialog is a small modal that asks the user to confirm an action.
// Clicking the backdrop or pressing Escape cancels (unless busy). The app's
// shared confirm modal, now built on the Oxygen/MUI Dialog.
export function ConfirmDialog({ title, message, confirmLabel, danger, busy, onConfirm, onCancel }: Props) {
  return (
    <Dialog
      open
      onClose={() => !busy && onCancel()}
      aria-labelledby="confirm-dialog-title"
      maxWidth="xs"
      fullWidth
    >
      <DialogTitle id="confirm-dialog-title">{title}</DialogTitle>
      <DialogContent sx={{ color: "text.secondary" }}>{message}</DialogContent>
      <DialogActions sx={{ px: 3, pb: 2 }}>
        <Button variant="text" color="inherit" onClick={onCancel} disabled={busy}>
          Cancel
        </Button>
        <Button
          variant="contained"
          color={danger ? "error" : "primary"}
          onClick={onConfirm}
          disabled={busy}
        >
          {busy ? "Working…" : confirmLabel}
        </Button>
      </DialogActions>
    </Dialog>
  );
}

export interface ConfirmRequest {
  title: string;
  message: React.ReactNode;
  confirmLabel: string;
  // danger defaults to true — the hook exists for destructive actions.
  danger?: boolean;
  onConfirm: () => void;
}

// useConfirmAction is the app-wide guard in front of every remove/delete control:
// nothing destructive fires straight from a click. It returns the modal node to
// render (null while idle) and an `ask` that opens it; the action runs only once
// the user accepts. The modal closes as the action fires, so callers keep their
// own pending/error UI (a mutation's isPending, an Alert) exactly as before.
export function useConfirmAction(): [React.ReactNode, (req: ConfirmRequest) => void] {
  const [request, setRequest] = useState<ConfirmRequest | null>(null);

  const node = request ? (
    <ConfirmDialog
      title={request.title}
      message={request.message}
      confirmLabel={request.confirmLabel}
      danger={request.danger ?? true}
      onConfirm={() => {
        setRequest(null);
        request.onConfirm();
      }}
      onCancel={() => setRequest(null)}
    />
  ) : null;

  return [node, (req: ConfirmRequest) => setRequest(req)];
}
