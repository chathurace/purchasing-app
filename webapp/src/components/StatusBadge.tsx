import { Chip } from "@wso2/oxygen-ui";
import type { PRStatus } from "../types/api";

const LABELS: Record<PRStatus, string> = {
  submitted: "Submitted",
  under_review: "Under review",
  vendor_selected: "Vendor selected",
  contract_prepared: "Contract prepared",
  order_signed: "Order signed",
  completed: "Completed",
  rejected: "Rejected",
  cancelled: "Cancelled",
};

// Map each PR status onto an MUI semantic palette colour (the theme renders
// these as flat, outlined pills — see the shared Chip usage across the app).
type ChipColor = "default" | "primary" | "secondary" | "success" | "error" | "info" | "warning";

const COLORS: Record<PRStatus, ChipColor> = {
  submitted: "info",
  under_review: "warning",
  vendor_selected: "secondary",
  contract_prepared: "secondary",
  order_signed: "success",
  completed: "success",
  rejected: "error",
  cancelled: "default",
};

export function StatusBadge({ status }: { status: PRStatus }) {
  return (
    <Chip
      size="small"
      variant="outlined"
      color={COLORS[status] ?? "default"}
      label={LABELS[status] ?? status}
    />
  );
}
