import { Chip } from "@wso2/oxygen-ui";
import type { ContractStatus, InvoiceStatus, QuotationStatus } from "../types/api";

type AnyStatus = QuotationStatus | ContractStatus | InvoiceStatus;

const LABELS: Record<AnyStatus, string> = {
  // Quotation
  received: "Received",
  under_evaluation: "Under evaluation",
  selected: "Selected",
  // Contract
  draft: "Draft",
  approved: "Approved",
  signed: "Signed",
  // Invoice (received/approved shared with above); paid is invoice-only
  paid: "Paid",
  // shared: rejected
  rejected: "Rejected",
};

type ChipColor = "default" | "primary" | "secondary" | "success" | "error" | "info" | "warning";

const COLORS: Record<AnyStatus, ChipColor> = {
  received: "info",
  under_evaluation: "warning",
  selected: "success",
  draft: "default",
  approved: "success",
  signed: "success",
  paid: "success",
  rejected: "error",
};

export function EntityStatusBadge({ status }: { status: AnyStatus }) {
  return (
    <Chip
      size="small"
      variant="outlined"
      color={COLORS[status] ?? "default"}
      label={LABELS[status] ?? status}
    />
  );
}
