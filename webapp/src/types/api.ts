export type Role = "staff" | "procurement" | "procurement_admin" | "admin" | "legal" | "security";

export type PRStatus =
  | "submitted"
  | "under_review"
  | "vendor_selected"
  | "contract_prepared"
  | "order_signed"
  | "completed"
  | "rejected"
  | "cancelled";

export type QuotationStatus = "received" | "under_evaluation" | "selected" | "rejected";

// A contract is draft until its signed PDF is attached, then signed. (rejected is
// only reached when the owning PR is rejected.) Contract review/approval was
// removed — approvals happen on the purchase request.
export type ContractStatus = "draft" | "signed" | "rejected";

export type InvoiceStatus = "received" | "approved" | "paid";

export interface Me {
  id: number;
  sub: string;
  email: string;
  name: string;
  roles: Role[];
  // True when the user is an approver on any PR they didn't submit (a named
  // approver, or a budget/legal/security recommendation-card actor). Roles alone
  // can't tell — budget owners and named approvers hold no distinguishing role.
  // Drives the Approvals/Quotations/Contracts nav tabs.
  is_approver: boolean;
}

// Roles an admin may grant/revoke. `staff` is a permanent baseline (auto-granted
// on first login, never removable) and so is excluded — mirrors model.AssignableRoles.
export const ASSIGNABLE_ROLES: Role[] = ["procurement", "procurement_admin", "admin", "legal", "security"];

export const ROLE_LABELS: Record<Role, string> = {
  staff: "Staff",
  procurement: "Procurement",
  procurement_admin: "Procurement admin",
  admin: "Admin",
  legal: "Legal",
  security: "Security",
};

// AdminUser is the user-management view of a user: roles plus lifecycle flags.
export interface AdminUser {
  id: number;
  email: string;
  name: string;
  is_active: boolean;
  pending: boolean; // never logged in (no OIDC subject yet)
  roles: Role[];
  created_at: string;
}

export interface UserSummary {
  id: number;
  email: string;
  name: string;
}

// A team (Legal / Security / Procurement). Membership is the member_role: a user is a
// member iff they hold that role, so adding/removing a member grants/revokes it.
// admin_members are the users holding the team's admin-role variant (only the
// Procurement team has one — procurement_admin); they count as members for display
// but are managed from the Users page, not the team card.
export interface Team {
  id: number;
  key: string;
  name: string;
  member_role: Role;
  team_email: string;
  members: UserSummary[];
  admin_members: UserSummary[];
  created_at: string;
  updated_at: string;
}

export interface Item {
  id?: number;
  description: string;
  quantity: number;
  position?: number;
}

export interface Link {
  id?: number;
  url: string;
  label: string;
  position?: number;
}

export interface Document {
  id: number;
  filename: string;
  content_type: string;
  size_bytes: number;
  // Optional free-text note (used by the contract card for each draft/signed PDF).
  notes: string;
  uploaded_by?: number | null;
  created_at: string;
}

export type ApprovalStatus = "pending" | "approved" | "rejected";

export const APPROVAL_STATUS_LABELS: Record<ApprovalStatus, string> = {
  pending: "Pending",
  approved: "Approved",
  rejected: "Rejected",
};

// PRApproval is one named approver's decision on a purchase request.
export interface PRApproval {
  id: number;
  purchase_request_id: number;
  approver_id: number;
  status: ApprovalStatus;
  comment: string;
  decided_at: string | null;
  created_at: string;
  updated_at: string;
  approver?: UserSummary | null;
}

// --- Procurement recommendation (one per PR) ---

export type RecApprovalType = "budget" | "legal" | "security";

export const REC_APPROVAL_TYPES: RecApprovalType[] = ["budget", "legal", "security"];

export const REC_APPROVAL_LABELS: Record<RecApprovalType, string> = {
  budget: "Budget owner",
  legal: "Legal",
  security: "Security",
};

export interface RecComment {
  id: number;
  author_id: number;
  author?: UserSummary | null;
  comment: string;
  created_at: string;
  documents: Document[];
}

export interface RecApproval {
  approval_type: RecApprovalType;
  approved: boolean;
  approved_by?: number | null;
  approver?: UserSummary | null;
  approved_at?: string | null;
  // The team member responsible for this card (legal/security only). Only the
  // assignee may approve; the budget card has none.
  assignee_id?: number | null;
  assignee?: UserSummary | null;
  comments: RecComment[];
  // Per-caller capability flags (server-computed): whether the current user may
  // comment on, approve, or (re)assign this card.
  can_comment: boolean;
  can_approve: boolean;
  can_assign: boolean;
}

export interface Recommendation {
  id: number;
  purchase_request_id: number;
  vendor_id: number;
  vendor?: Vendor | null;
  description: string;
  // Commercial details, mirroring the PR's commercial section.
  estimated_value: number;
  currency: string;
  engagement_type: string;
  created_at: string;
  updated_at: string;
  approvals: RecApproval[];
  // The approval types the current user may toggle/comment on (server-computed).
  my_actionable_types: RecApprovalType[];
  // The contract attached to this recommendation (the optional contract card), or null.
  contract_id: number | null;
  contract?: Contract | null;
  // The optional RFI (request for information) raised with the vendor: a
  // description plus PDF attachments. Present when description is non-empty or
  // there is at least one attachment.
  rfi_description: string;
  rfi_documents: Document[];
}

export interface RecommendationInput {
  vendor_id: number;
  description: string;
  estimated_value: number;
  currency: string;
  engagement_type: string;
  required_types: RecApprovalType[];
}

// --- Requisition form (WSO2 SOP-85000) ---

export type YesNo = "yes" | "no" | "";
// Within-budget answer allows an explicit "I don't know yet".
export type YesNoUnknown = YesNo | "unknown";
export type PRCategory = "IT" | "NON-IT" | "";

// PRDetails is the structured form data stored in the PR's JSONB `details` blob
// (everything not promoted to a core column).
export interface PRDetails {
  requester_name?: string;
  requester_email?: string;
  date?: string;
  business_justification?: string;
  // IT solution
  it_category?: string;
  it_product?: string;
  it_description?: string;
  it_plan?: string;
  it_users?: string;
  it_admins?: string;
  it_usage?: string;
  sec_sensitive?: YesNo;
  sec_external_pii?: YesNo;
  sec_external_pii_detail?: string;
  sec_employee_pii?: YesNo;
  sec_integrates?: YesNo;
  sec_integration_detail?: string;
  // Non-IT solution
  nit_category?: string;
  nit_description?: string;
  nit_specs?: string;
  // Vendor
  supplier_name?: string;
  supplier_website?: string;
  supplier_contact?: string;
  supplier_email?: string;
  supplier_existing?: YesNo;
  // Set when the requester picked an existing vendor from the dropdown (vs.
  // typing a new supplier, which is never added to the vendor master).
  supplier_vendor_id?: number | null;
  engagement_type?: string;
  within_budget?: YesNoUnknown;
  // Budget coding
  budget_category?: string;
  budget_product?: string;
  budget_region?: string;
  engagement_code?: string;
  notes?: string;
}

// The arrays below are fallback defaults that mirror the values seeded into the
// config_options table (migration 024). The runtime source of truth is the
// Settings page (/config/options); these are used only while the lookup loads or
// if a list has been emptied. See hooks/useConfigOptions.ts.
export const WSO2_ENTITIES = [
  "WSO2 Lanka Private Limited", "WSO2 LLC (USA)", "WSO2 UK Limited",
  "WSO2 Australia", "WSO2 India", "Other",
];

export const IT_CATEGORIES = [
  "Software licences & subscriptions (incl. renewals)",
  "SaaS platform / cloud service", "IT support services",
  "Hosting services & domain names", "IT hardware & equipment",
  "Free / open-source tool (approval required)", "Other IT solution",
];

export const NONIT_CATEGORIES = [
  "Fixed assets & hardware equipment",
  "Insurance (Medical, Life, Public Liability)",
  "Seasonal gifts, hampers, employee welcome packs",
  "Corporate merchandise & bulk printing",
  "Facilities — Soft services (security, janitorial)",
  "Facilities — Hard services (engineering, maintenance)",
  "Travel & accommodation", "Event management", "Other non-IT solution",
];

export const ENGAGEMENT_TYPES = [
  "One-time purchase", "Monthly subscription", "Annual subscription",
  "Multi-year agreement", "Other",
];

export const REQ_CURRENCIES = ["USD", "LKR", "GBP", "AUD", "INR", "EUR"];

export const BUDGET_CATEGORIES = [
  "Software & Subscriptions", "Hardware & Equipment", "Cloud & Hosting",
  "Professional Services", "Marketing & Events", "Facilities & Office",
  "HR & People", "Travel & Accommodation", "Insurance", "Other",
];

export const PRODUCTS = [
  "WSO2 API Manager", "WSO2 Identity Server", "WSO2 MI / Integration",
  "Choreo", "Asgardeo", "Ballerina", "Cross-product / Platform", "Not product-specific",
];

export const REGIONS = [
  "APAC — Sri Lanka", "APAC — India", "APAC — Australia", "APAC — Other",
  "Americas — USA", "Americas — Other", "EMEA — UK", "EMEA — Europe",
  "EMEA — Other", "Global / Multi-region",
];

export interface PurchaseRequest {
  id: number;
  // Human-readable reference (PR-YYYY-NNNNNNN) assigned at submission; null for
  // requests created before the feature (display falls back to the system id).
  reference?: string | null;
  title: string;
  requester_id: number;
  budget_unit_id: number | null;
  comments: string;
  status: PRStatus;
  rejection_reason: string;
  created_at: string;
  updated_at: string;
  items: Item[];
  links: Link[];
  documents: Document[];
  requester?: UserSummary | null;
  approvals?: PRApproval[];
  approvals_total: number;
  approvals_approved: number;
  // The current user's own approval status on this request (list reads), or
  // null/absent when they are not an approver.
  my_approval_status?: ApprovalStatus | null;
  // The caller's unified approval state across BOTH systems (named approvals +
  // budget/legal/security recommendation cards): pending | approved | rejected.
  // Drives the Approvals-tab filter; only meaningful on the scope=approvals list.
  my_approval_state?: ApprovalStatus | null;
  // The procurement recommendation (detail reads only), or null when none.
  recommendation?: Recommendation | null;
  // Team lead approval: the requester names a team lead by email who must approve
  // the PR before procurement can see or act on it. Until then the PR is hidden from
  // everyone but the requester, the team lead, and admins.
  team_lead_email: string;
  team_lead_status: ApprovalStatus;
  team_lead_notes: string;
  team_lead_decided_at: string | null;
  team_lead_decided_by: number | null;
  // Per-caller flag (detail reads only): true when the caller may record the
  // team lead decision (they are the team lead or an admin).
  my_team_lead_actionable?: boolean;
  // PR assignment: after team-lead approval, a procurement user must be assigned
  // before procurement work can start. assignee + collaborators are the procurement
  // users who may act on the PR. assignee is included on list reads (for the badge);
  // collaborators only on detail reads.
  assignee_id?: number | null;
  assignee?: UserSummary | null;
  assigned_at?: string | null;
  collaborators?: UserSummary[];
  // Per-caller flags (detail reads only): my_can_assign = may change the assignee;
  // my_can_manage_collaborators = may add/remove collaborators; my_can_work = may
  // perform procurement work on this PR (assigned, and assignee/collaborator/admin).
  my_can_assign?: boolean;
  my_can_manage_collaborators?: boolean;
  my_can_work?: boolean;
  // Requisition fields — core columns (list + detail); budget_approver_*/details
  // are detail-only.
  team: string;
  entity: string;
  category: PRCategory;
  estimated_value: number;
  currency: string;
  budget_approver_name?: string;
  budget_approver_email?: string;
  details?: PRDetails;
}

// allApproved reports whether every named approver has approved the request.
export function allApproved(pr: { approvals_total: number; approvals_approved: number }): boolean {
  return pr.approvals_total === 0 || pr.approvals_approved === pr.approvals_total;
}

export interface Vendor {
  id: number;
  name: string;
  contact_name: string;
  email: string;
  phone: string;
  notes: string;
  is_active: boolean;
  tax_id: string;
  address_line: string;
  city: string;
  postal_code: string;
  country: string;
  website: string;
  registered: boolean;
  created_at: string;
  updated_at: string;
}

export interface VendorInput {
  name: string;
  contact_name: string;
  email: string;
  phone: string;
  notes: string;
  is_active: boolean;
  tax_id: string;
  address_line: string;
  city: string;
  postal_code: string;
  country: string;
  website: string;
  registered: boolean;
}

// VendorLookup is the minimal vendor summary served to any authenticated user
// for the purchase-request "proposed supplier" dropdown.
export interface VendorLookup {
  id: number;
  name: string;
  website: string;
  contact_name: string;
  email: string;
  registered: boolean;
}

export interface VendorUsage {
  quotations: number;
  contracts: number;
  grns: number;
  invoices: number;
}

// A budget unit's approval bracket: an inclusive value range (max null =
// unbounded) with one or more approvers. Any approver of the resolved bracket
// may approve the budget card.
export interface BudgetUnitBracket {
  id: number;
  position: number;
  currency: string;
  min_value: number;
  max_value: number | null;
  approvers: UserSummary[];
}

export interface BudgetUnit {
  id: number;
  code: string;
  name: string;
  description: string;
  budget: number;
  currency: string;
  is_active: boolean;
  // The catch-all approver used when a PR matches no bracket.
  default_approver_id: number | null;
  default_approver?: UserSummary | null;
  created_at: string;
  updated_at: string;
  brackets: BudgetUnitBracket[];
}

// Bracket shape for create/update: currency + min/max + the approver ids.
// Brackets are ordered by their position in the array (first match wins).
export interface BudgetUnitBracketInput {
  currency: string;
  min_value: number;
  max_value: number | null;
  approver_ids: number[];
}

export interface BudgetUnitInput {
  code: string;
  name: string;
  description: string;
  budget: number;
  currency: string;
  is_active: boolean;
  default_approver_id: number | null;
  brackets: BudgetUnitBracketInput[];
}

// BudgetUnitSummary is the lightweight shape returned by the lookup endpoint,
// used to populate the budget-unit dropdown on a purchase request.
export interface BudgetUnitSummary {
  id: number;
  code: string;
  name: string;
  currency: string;
}

export interface BudgetUnitUsage {
  purchase_requests: number;
}

// CurrencyTotal is an amount paired with its currency (never summed across
// currencies).
export interface CurrencyTotal {
  currency: string;
  amount: number;
}

// BudgetUnitInvoiceCategory summarises one invoice status bucket: how many
// invoices allocate to the budget unit and the allocated totals per currency.
export interface BudgetUnitInvoiceCategory {
  count: number;
  totals: CurrencyTotal[];
}

// BudgetUnitInvoiceSummary buckets a budget unit's allocated invoices by status
// (pending = received), each with the budget unit's allocated share.
export interface BudgetUnitInvoiceSummary {
  pending: BudgetUnitInvoiceCategory;
  approved: BudgetUnitInvoiceCategory;
  paid: BudgetUnitInvoiceCategory;
}

export interface QuotationItem {
  id?: number;
  description: string;
  quantity: number;
  unit_price: number;
  position?: number;
}

export interface Quotation {
  id: number;
  purchase_request_id: number;
  vendor_id: number;
  total_amount: number;
  currency: string;
  valid_until: string | null;
  notes: string;
  status: QuotationStatus;
  created_at: string;
  updated_at: string;
  items: QuotationItem[];
  vendor?: Vendor | null;
  // documents holds the other supporting documents; the single primary quotation
  // PDF is exposed separately as quotation_document.
  documents?: Document[];
  quotation_document_id?: number | null;
  quotation_document?: Document | null;
}

export interface QuotationInput {
  vendor_id: number;
  total_amount: number;
  currency: string;
  valid_until: string | null;
  notes: string;
  items: { description: string; quantity: number; unit_price: number }[];
}

export interface Contract {
  id: number;
  purchase_request_id: number;
  quotation_id: number | null;
  vendor_id: number;
  title: string;
  total_amount: number;
  currency: string;
  terms: string;
  status: ContractStatus;
  signed_document_id: number | null;
  created_at: string;
  updated_at: string;
  vendor?: Vendor | null;
  // Draft-contract PDFs (each with its own notes). The signed PDF is separate.
  documents?: Document[];
  signed_document?: Document | null;
  // Sum of this contract's invoice totals in its own currency (detail reads only).
  invoiced_total?: number;
  // The owning PR's budget unit, if any — used to default an invoice's allocation.
  budget_unit?: BudgetUnitSummary | null;
}

export interface ContractInput {
  title: string;
  total_amount: number;
  currency: string;
  terms: string;
}

// --- Fulfillment: GRNs and invoices (children of a signed contract) ---

export interface GRNItem {
  id?: number;
  description: string;
  quantity: number;
  position?: number;
}

export interface GRN {
  id: number;
  contract_id: number;
  purchase_request_id: number;
  vendor_id: number;
  received_date: string; // YYYY-MM-DD
  received_by: string;
  note: string;
  created_at: string;
  updated_at: string;
  items: GRNItem[];
  vendor?: Vendor | null;
  documents?: Document[];
}

export interface GRNInput {
  received_date: string;
  received_by: string;
  note: string;
  items: { description: string; quantity: number }[];
}

export interface InvoiceItem {
  id?: number;
  description: string;
  quantity: number;
  unit_price: number;
  position?: number;
}

// AllocationMode controls how an invoice is split across budget units: either
// entirely by percentage (values sum to 100) or by absolute amount (values sum
// to the invoice total).
export type AllocationMode = "percentage" | "amount";

// CostAllocation is an invoice's cost split to one budget unit. value is a
// percentage or an amount per the invoice's allocation_mode; amount is the
// resolved amount in the invoice currency (server-computed, read-only).
export interface CostAllocation {
  id?: number;
  budget_unit_id: number;
  value: number;
  position?: number;
  budget_unit?: BudgetUnitSummary | null;
  amount?: number;
}

// CostAllocationInput is the writable shape sent on create/update.
export interface CostAllocationInput {
  budget_unit_id: number;
  value: number;
}

export interface Invoice {
  id: number;
  contract_id: number;
  purchase_request_id: number;
  vendor_id: number;
  vendor_invoice_no: string;
  invoice_date: string; // YYYY-MM-DD
  due_date: string | null;
  total_amount: number; // effective total: entered_total if set, else line-items sum
  entered_total: number | null; // directly-entered total, or null when derived from items
  currency: string;
  status: InvoiceStatus;
  note: string;
  paid_date: string | null;
  approved_at: string | null;
  created_at: string;
  updated_at: string;
  allocation_mode: AllocationMode;
  items: InvoiceItem[];
  cost_allocations: CostAllocation[];
  vendor?: Vendor | null;
  approver?: UserSummary | null;
  documents?: Document[];
}

export interface InvoiceInput {
  vendor_invoice_no: string;
  invoice_date: string;
  due_date: string | null;
  currency: string;
  note: string;
  allocation_mode: AllocationMode;
  // Directly-entered invoice total. When non-null it is the invoice value (takes
  // priority over the line items); null derives the total from the line items.
  entered_total: number | null;
  items: { description: string; quantity: number; unit_price: number }[];
  cost_allocations: CostAllocationInput[];
}

// PRSummary is the lightweight purchase-request reference returned in a related
// documents bundle (full PurchaseRequest is not needed for cross-links).
export interface PRSummary {
  id: number;
  reference?: string | null;
  title: string;
  status: PRStatus;
}

// RelatedDocuments is every procurement record linked to one case (the PR and
// all of its quotations and contracts), used to cross-link detail pages.
export interface RelatedDocuments {
  purchase_request: PRSummary;
  quotations: Quotation[];
  contracts: Contract[];
  grns: GRN[];
  invoices: Invoice[];
}

export interface PurchaseRequestInput {
  title: string;
  budget_unit_id: number | null;
  comments: string;
  items: { description: string; quantity: number }[];
  links: { url: string; label: string }[];
  // Requisition form fields.
  team: string;
  entity: string;
  category: PRCategory;
  estimated_value: number;
  currency: string;
  budget_approver_name: string;
  budget_approver_email: string;
  details: PRDetails;
  // Team lead who must approve the PR before procurement can act on it. Required.
  team_lead_email: string;
}

// EDITABLE_STATUSES mirrors the backend model.EditableStatuses.
export const EDITABLE_STATUSES: PRStatus[] = ["submitted", "under_review"];

// prReference returns the stored human-readable reference when present. The
// reference is a display label whose format may change over time, so it is kept
// separate from the immutable system id; when no reference exists (legacy PRs)
// we fall back to the raw system id (#42) rather than synthesizing a reference.
export function prReference(pr: { id: number; reference?: string | null }): string {
  return pr.reference || `#${pr.id}`;
}

export function quoRef(id: number): string {
  return "QUO-" + String(id).padStart(6, "0");
}

export function conRef(id: number): string {
  return "CON-" + String(id).padStart(6, "0");
}

export function vendorRef(id: number): string {
  return "VEN-" + String(id).padStart(6, "0");
}

export function budgetUnitRef(id: number): string {
  return "BU-" + String(id).padStart(6, "0");
}

// A fresh, active budget-unit draft. Shared by the management list (inline add)
// and detail (edit) pages. Starts with one unbounded, no-approver bracket.
export const emptyBudgetUnit: BudgetUnitInput = {
  code: "",
  name: "",
  description: "",
  budget: 0,
  currency: "",
  is_active: true,
  default_approver_id: null,
  brackets: [],
};

export function grnRef(id: number): string {
  return "GRN-" + String(id).padStart(6, "0");
}

export function invRef(id: number): string {
  return "INV-" + String(id).padStart(6, "0");
}

export const INVOICE_STATUS_LABELS: Record<InvoiceStatus, string> = {
  received: "Received",
  approved: "Approved",
  paid: "Paid",
};

// invoiceItemsTotal sums the line items, mirroring the server-derived total used
// for the over-billing check.
export function invoiceItemsTotal(items: { quantity: number; unit_price: number }[]): number {
  return items.reduce((sum, it) => sum + (it.quantity || 0) * (it.unit_price || 0), 0);
}

// invoiceEffectiveTotal is the invoice value the server persists: the entered
// total when supplied (it takes priority), otherwise the line-items sum.
export function invoiceEffectiveTotal(input: {
  entered_total: number | null;
  items: { quantity: number; unit_price: number }[];
}): number {
  return input.entered_total != null ? input.entered_total : invoiceItemsTotal(input.items);
}

// allocationsSum totals the raw allocation values (percentages or amounts).
export function allocationsSum(allocs: { value: number }[]): number {
  return allocs.reduce((s, a) => s + (a.value || 0), 0);
}

// allocationsValid mirrors the server rule: at least one allocation, each with a
// distinct budget unit, summing to 100 (percentage mode) or the invoice total
// (amount mode), within a small rounding tolerance.
export function allocationsValid(
  mode: AllocationMode,
  allocs: { budget_unit_id: number; value: number }[],
  total: number,
): boolean {
  if (allocs.length === 0) return false;
  if (allocs.some((a) => !a.budget_unit_id)) return false;
  const ids = allocs.map((a) => a.budget_unit_id);
  if (new Set(ids).size !== ids.length) return false;
  const target = mode === "percentage" ? 100 : total;
  return Math.abs(allocationsSum(allocs) - target) <= 0.01;
}

// formatMoney renders an amount with its currency code. Amounts of differing
// currencies must never be summed.
export function formatMoney(amount: number, currency: string): string {
  const n = (amount ?? 0).toLocaleString(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  });
  return `${n} ${currency || ""}`.trim();
}
