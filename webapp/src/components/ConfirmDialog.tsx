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
