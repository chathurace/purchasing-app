export type Role =
  | "staff"
  | "procurement"
  | "procurement_admin"
  | "admin"
  | "legal"
  | "security"
  | "compliance";

export type PRStatus =
  | "submitted"
  | "under_review"
  | "vendor_selected"
  | "contract_prepared"
  | "order_signed"
  | "completed"
  | "rejected"
  | "cancelled";

// PR_STATUS_LABELS / PR_STATUSES drive the status dropdown on the Purchase-requests
// filter bar (and any other status selector). Order matches the workflow.
export const PR_STATUS_LABELS: Record<PRStatus, string> = {
  submitted: "Submitted",
  under_review: "Under review",
  vendor_selected: "Vendor selected",
  contract_prepared: "Contract prepared",
  order_signed: "Order signed",
  completed: "Completed",
  rejected: "Rejected",
  cancelled: "Cancelled",
};

export const PR_STATUSES = Object.keys(PR_STATUS_LABELS) as PRStatus[];

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
  // approver, or a budget/legal/security/compliance recommendation-card actor). Roles alone
  // can't tell — budget owners and named approvers hold no distinguishing role.
  // Drives the Approvals/Quotations/Contracts nav tabs.
  is_approver: boolean;
}

// Roles an admin may grant/revoke. `staff` is a permanent baseline (auto-granted
// on first login, never removable) and so is excluded — mirrors model.AssignableRoles.
export const ASSIGNABLE_ROLES: Role[] = [
  "procurement",
  "procurement_admin",
  "admin",
  "legal",
  "security",
  "compliance",
];

export const ROLE_LABELS: Record<Role, string> = {
  staff: "Staff",
  procurement: "Procurement",
  procurement_admin: "Procurement admin",
  admin: "Admin",
  legal: "Legal",
  security: "Security",
  compliance: "Compliance",
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

// ── Audit / process events (admin events view) ──────────────────────────────
// Two append-only logs. Process events are PR business-process tasks; audit
// events are non-process master-data/admin mutations. Both carry an immutable
// actor_email snapshot plus the actor's current name (joined at read time).

export interface ProcessEvent {
  id: number;
  purchase_request_id: number;
  action: string;
  qualifier: string;
  actor_id: number | null;
  actor_email: string;
  actor_name: string;
  created_at: string;
}

export interface AuditEvent {
  id: number;
  action: string;
  qualifier: string;
  entity_type: string;
  entity_id: number | null;
  detail: string;
  actor_id: number | null;
  actor_email: string;
  actor_name: string;
  created_at: string;
}

// The fixed action catalogs (from the Go source of truth) backing the two
// action-filter dropdowns.
export interface EventActions {
  process: string[];
  audit: string[];
}

// Shared query filters for an events read; pr_id applies only to process events.
export interface EventFilters {
  actor?: string;
  action?: string;
  from?: string;
  to?: string;
  pr_id?: string;
}

// A directory entry from the connected identity server (SCIM), used for
// name/email autocomplete. Not necessarily a provisioned app user — they may
// never have logged in — so it carries no id.
export interface DirectoryUser {
  name: string;
  email: string;
}

// A team (Legal / Security / Compliance / Procurement). Membership is the member_role: a user is a
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

export type RecApprovalType = "budget" | "legal" | "security" | "compliance";

export const REC_APPROVAL_TYPES: RecApprovalType[] = ["budget", "legal", "security", "compliance"];

export const REC_APPROVAL_LABELS: Record<RecApprovalType, string> = {
  budget: "Budget owner",
  legal: "Legal",
  security: "Security",
  compliance: "Compliance",
};

export interface RecComment {
  id: number;
  author_id: number;
  author?: UserSummary | null;
  comment: string;
  created_at: string;
  documents: Document[];
}

export type BudgetStepDecision = "pending" | "approved" | "rejected";

// One step in the budget card's serial approval chain. Step 1 (is_base) is
// governed by the PR's budget unit (no named approver); additional steps name a
// specific approver by email and are decided one after the other.
export interface BudgetStep {
  id: number;
  position: number;
  is_base: boolean;
  approver_name: string;
  approver_email: string;
  decision: BudgetStepDecision;
  decided_by?: number | null;
  decider?: UserSummary | null;
  decided_at?: string | null;
  comments: RecComment[];
  // Per-caller flags (server-computed): whether the current user may decide this
  // step right now (serial + identity gated) / may edit or remove it (procurement).
  can_decide: boolean;
  can_manage: boolean;
}

export interface RecApproval {
  approval_type: RecApprovalType;
  approved: boolean;
  approved_by?: number | null;
  approver?: UserSummary | null;
  approved_at?: string | null;
  // The team member responsible for this card (team cards only, not budget). Only the
  // assignee may approve; the budget card has none.
  assignee_id?: number | null;
  assignee?: UserSummary | null;
  comments: RecComment[];
  // Per-caller capability flags (server-computed): whether the current user may
  // comment on, approve, or (re)assign this card.
  can_comment: boolean;
  can_approve: boolean;
  can_assign: boolean;
  // The serial budget approval chain — populated only for the budget card. The
  // card's approved/approver above are a projection (approved iff every step is).
  budget_steps?: BudgetStep[];
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
export type PRCategory = "IT" | "NON-IT" | "EVENTS" | "";

// PRDetails is the structured form data stored in the PR's JSONB `details` blob
// (everything not promoted to a core column).
export interface PRDetails {
  requester_name?: string;
  requester_email?: string;
  date?: string;
  business_justification?: string;
  // IT solution
  it_category?: string; // legacy — no longer collected (kept for old rows)
  it_product?: string;
  it_description?: string;
  it_plan?: string;
  it_users?: string; // legacy free-text count (kept for old rows)
  it_admins?: string; // legacy free-text (kept for old rows)
  it_usage?: string; // stores the usage-period select value
  it_user_count?: string;
  it_user_names?: string[];
  it_admin_count?: string;
  it_admin_names?: string[];
  sec_sensitive?: YesNo;
  sec_external_pii?: YesNo;
  sec_external_pii_detail?: string;
  sec_employee_pii?: YesNo;
  sec_employee_pii_detail?: string;
  sec_integrates?: YesNo;
  sec_integration_detail?: string; // legacy (kept for old rows)
  sec_integration_systems?: string;
  sec_integration_kind?: string;
  sec_vendor_docs?: YesNo;
  sec_vendor_docs_link?: string;
  // Non-IT solution
  nit_category?: string; // legacy — no longer collected (kept for old rows)
  nit_description?: string;
  nit_specs?: string;
  // Vendor
  supplier_name?: string;
  supplier_website?: string;
  supplier_contact?: string;
  supplier_email?: string;
  supplier_phone?: string;
  supplier_existing?: YesNo; // legacy (kept for old rows)
  // Set when the requester picked an existing vendor from the dropdown (vs.
  // typing a new supplier, which is never added to the vendor master).
  supplier_vendor_id?: number | null;
  engagement_type?: string; // legacy (kept for old rows)
  within_budget?: YesNoUnknown; // legacy (kept for old rows)
  // Budget coding (free-text in the ProQ form)
  business_unit?: string;
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

// Procurement triage priority, set from the PR's assignment card. P3 is the
// default every PR is created with — the requester never picks one.
export type PRPriority = "P1" | "P2" | "P3";

// PR_PRIORITIES is the ordered (highest first) option list for the picker; the
// colour is the traffic-light convention the priorities were defined with.
export const PR_PRIORITIES: {
  value: PRPriority;
  label: string;
  color: "error" | "warning" | "success";
}[] = [
  { value: "P1", label: "P1 — High", color: "error" },
  { value: "P2", label: "P2 — Medium", color: "warning" },
  { value: "P3", label: "P3 — Normal", color: "success" },
];

// prPriority normalizes a PR's priority for display, defaulting rows that predate
// the column (or a summary read that omitted it) to P3.
export function prPriority(pr: { priority?: PRPriority | null }): PRPriority {
  return pr.priority ?? "P3";
}

export function prPriorityColor(p: PRPriority): "error" | "warning" | "success" {
  return PR_PRIORITIES.find((o) => o.value === p)?.color ?? "success";
}

export interface PurchaseRequest {
  id: number;
  // Human-readable reference (PR-YYYY-NNNNNNN) assigned at submission; null for
  // requests created before the feature (display falls back to the system id).
  reference?: string | null;
  title: string;
  requester_id: number;
  business_unit_id: number | null;
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
  // budget/legal/security/compliance recommendation cards): pending | approved | rejected.
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
  // Procurement triage priority (P1 highest .. P3 default), on list + detail
  // reads. my_can_set_priority (detail reads only) = the caller may change it.
  priority: PRPriority;
  my_can_set_priority?: boolean;
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

export interface BusinessUnit {
  id: number;
  name: string;
  description: string;
  is_active: boolean;
  created_at: string;
  updated_at: string;
  // The flat, ordered set of qualified budget approvers.
  approvers: UserSummary[];
}

export interface BusinessUnitInput {
  name: string;
  description: string;
  is_active: boolean;
  approver_ids: number[];
}

// BusinessUnitSummary is the lightweight shape returned by the lookup endpoint,
// used to populate the business-unit dropdown on a purchase request.
export interface BusinessUnitSummary {
  id: number;
  name: string;
}

export interface BusinessUnitUsage {
  purchase_requests: number;
}

// CurrencyTotal is an amount paired with its currency (never summed across
// currencies).
export interface CurrencyTotal {
  currency: string;
  amount: number;
}

// BusinessUnitInvoiceCategory summarises one invoice status bucket: how many
// invoices allocate to the business unit and the allocated totals per currency.
export interface BusinessUnitInvoiceCategory {
  count: number;
  totals: CurrencyTotal[];
}

// BusinessUnitInvoiceSummary buckets a business unit's allocated invoices by
// status (pending = received), each with the business unit's allocated share.
export interface BusinessUnitInvoiceSummary {
  pending: BusinessUnitInvoiceCategory;
  approved: BusinessUnitInvoiceCategory;
  paid: BusinessUnitInvoiceCategory;
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
  // documents holds the other supporting documents; the two primary quotation
  // PDFs — the initial quote from the vendor and the final (post-negotiation) one
  // — are exposed separately. Both are optional, and the final one never gates
  // selecting the quotation. Summary reads carry them too, so the PR-page cards
  // can render both slots without fetching the detail.
  documents?: Document[];
  initial_quotation_document_id?: number | null;
  initial_quotation_document?: Document | null;
  final_quotation_document_id?: number | null;
  final_quotation_document?: Document | null;
}

export interface QuotationInput {
  vendor_id: number;
  total_amount: number;
  currency: string;
  valid_until: string | null;
  notes: string;
  items: { description: string; quantity: number; unit_price: number }[];
  // extraction_id adopts a PDF already staged on the PR by the extraction flow as
  // this quotation's initial document, so the client doesn't re-upload it.
  extraction_id?: number | null;
}

// --- Quotation PDF extraction (Claude) ---

// ExtractionSuggestion is what the model read out of the PDF. Every field is
// optional: the prompt tells the model to omit rather than guess, so null means
// "not stated in the document" and must be shown as blank, never defaulted.
// A tax line as printed on the quotation. A list, not one field: split taxes
// (CGST + SGST, ICMS + PIS + COFINS) are normal and must stay separate.
export interface ExtractionTaxLine {
  label: string;
  rate: number | null; // percent, 20 for 20%
  amount: number | null;
}

// A non-tax charge line — shipping, freight, installation, handling.
export interface ExtractionCharge {
  label: string;
  amount: number | null;
}

export interface ExtractionSuggestion {
  vendor_name: string;
  currency: string;
  subtotal_amount: number | null;
  discount_amount: number | null;
  taxes: ExtractionTaxLine[];
  other_charges: ExtractionCharge[];
  total_amount: number | null;
  total_includes_tax: boolean | null;
  valid_until: string | null;
  quote_reference: string;
  quote_date: string | null;
  items: { description: string; quantity: number; unit_price: number }[];
  confidence: "high" | "medium" | "low" | "";
  notes: string;
}

export interface QuotationExtraction {
  id: number;
  purchase_request_id: number;
  document_id: number;
  quotation_id: number | null;
  status: "pending" | "succeeded" | "failed";
  model: string;
  error_message?: string;
  input_tokens: number;
  output_tokens: number;
  created_at: string;
  applied_at: string | null;
  filename?: string;
}

// VendorMatch ranks an existing vendor against the extracted vendor name. The
// model never returns an id — the user picks from these (or creates a vendor).
export interface VendorMatch {
  vendor: Vendor;
  score: number;
}

export interface ExtractionResponse {
  extraction: QuotationExtraction;
  suggestion?: ExtractionSuggestion;
  vendor_matches: VendorMatch[];
  // items_total is the sum of the line items, for the non-blocking mismatch
  // warning against the stated total (same pattern as invoice entered_total).
  items_total: number;
  // Sums of the tax and non-tax charge lines that stated an amount.
  tax_total: number;
  charges_total: number;
}

export interface ExtractionStatus {
  enabled: boolean;
  model: string;
  max_pdf_bytes: number;
}

// --- Quotation comparison (one per PR) ---
//
// The WSO2 vendor-quote comparison sheet: every vendor's initial quote beside its
// final (post-negotiation) one. Only the sheet's *inputs* are stored server-side
// (who generated it, the approved budget, the currency, the missing-final consent);
// every figure below is derived on each read from the quotations and what was read
// out of their PDFs, so editing a quotation updates the comparison.
// See docs/quotation-comparison.md.

export interface QuotationComparisonRecord {
  id: number;
  purchase_request_id: number;
  approved_budget: number | null;
  currency: string;
  use_initial_for_final: boolean;
  generated_at: string;
  generated_by: number | null;
  generator?: UserSummary | null;
  updated_at: string;
}

// Where a column's figures came from: read out of that slot's own PDF, taken from
// the quotation record (no read available), or the initial quote standing in for a
// final one the vendor never sent.
export type ComparisonSource = "pdf" | "record" | "initial";

export interface ComparisonMoneyLine {
  label: string;
  rate?: number | null;
  amount: number | null;
}

export interface ComparisonLineItem {
  description: string;
  quantity: number;
  unit_price: number;
  amount: number;
}

export interface ComparisonQuote {
  available: boolean;
  source: ComparisonSource;
  document_id?: number | null;
  filename?: string;
  currency: string;
  items: ComparisonLineItem[];
  items_total: number;
  subtotal: number | null;
  discount: number | null;
  taxes: ComparisonMoneyLine[];
  tax_total: number;
  charges: ComparisonMoneyLine[];
  charges_total: number;
  // grand_total is always tax-inclusive. Null when the source stated nothing — show
  // it blank, never 0.00.
  grand_total: number | null;
  stated_total: number | null;
  added_tax: boolean;
  derived_total: boolean;
  items_mismatch: boolean;
  valid_until: string | null;
  quote_reference?: string;
  confidence?: string;
  // applied: these are the figures the quotation record itself carries.
  applied: boolean;
}

// One item line with the initial and final quote side by side. Paired on the item
// description; a side the other quote doesn't list is null.
export interface ComparisonItemRow {
  description: string;
  initial_quantity: number | null;
  initial_unit_price: number | null;
  initial_amount: number | null;
  final_quantity: number | null;
  final_unit_price: number | null;
  final_amount: number | null;
}

export interface ComparisonVendor {
  quotation_id: number;
  vendor_id: number;
  vendor_name: string;
  notes: string;
  status: QuotationStatus;
  currency: string;
  initial: ComparisonQuote;
  final: ComparisonQuote;
  final_is_initial: boolean;
  items: ComparisonItemRow[];
  negotiated_saving: number | null;
  variance_vs_budget: number | null;
  saving_vs_highest: number | null;
  lowest: boolean;
}

export interface QuotationComparison {
  exists: boolean;
  comparison?: QuotationComparisonRecord | null;
  can_manage: boolean;
  currency: string;
  mixed_currency: boolean;
  approved_budget: number | null;
  vendors: ComparisonVendor[];
  // Vendors whose final quote could not be read from a PDF of their own — what the
  // confirmation modal warns about before generating, and what the card keeps
  // flagging afterwards.
  missing_final: string[];
  quotation_count: number;
}

export interface QuotationComparisonInput {
  approved_budget: number | null;
  currency: string;
  use_initial_for_final: boolean;
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
  // The owning PR's business unit, if any — used to default an invoice's allocation.
  business_unit?: BusinessUnitSummary | null;
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

// CostAllocation is an invoice's cost split to one business unit. value is a
// percentage or an amount per the invoice's allocation_mode; amount is the
// resolved amount in the invoice currency (server-computed, read-only).
export interface CostAllocation {
  id?: number;
  business_unit_id: number;
  value: number;
  position?: number;
  business_unit?: BusinessUnitSummary | null;
  amount?: number;
}

// CostAllocationInput is the writable shape sent on create/update.
export interface CostAllocationInput {
  business_unit_id: number;
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
  business_unit_id: number | null;
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

// A fresh, active business-unit draft. Shared by the management list (inline
// add) and detail (edit) pages. Starts with no approvers.
export const emptyBusinessUnit: BusinessUnitInput = {
  name: "",
  description: "",
  is_active: true,
  approver_ids: [],
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
// distinct business unit, summing to 100 (percentage mode) or the invoice total
// (amount mode), within a small rounding tolerance.
export function allocationsValid(
  mode: AllocationMode,
  allocs: { business_unit_id: number; value: number }[],
  total: number,
): boolean {
  if (allocs.length === 0) return false;
  if (allocs.some((a) => !a.business_unit_id)) return false;
  const ids = allocs.map((a) => a.business_unit_id);
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

// --- Home dashboard (GET /api/v1/home) ---

// HomeActivity is one recent process event on a PR, for the "latest activity" feed.
export interface HomeActivity {
  purchase_request_id: number;
  reference: string;
  title: string;
  action: string;
  qualifier: string;
  actor_email: string;
  created_at: string;
}

export interface StaffHome {
  my_requests_count: number;
  completed_count: number;
  recent_activity: HomeActivity[];
}

export interface ApprovalsHome {
  pending_count: number;
  completed_count: number;
  recent_activity: HomeActivity[];
}

export interface ProcurementHome {
  pending_count: number;
  awaiting_delivery_count: number;
  completed_count: number;
  recent_activity: HomeActivity[];
}

// HomeResponse carries whichever role blocks apply to the caller; staff is always
// present, approvals/procurement only when the caller qualifies.
export interface HomeResponse {
  staff?: StaffHome;
  approvals?: ApprovalsHome;
  procurement?: ProcurementHome;
}

// activityLabel humanizes a process event (action + qualifier) into a short past-
// tense phrase for the activity feed. Mirrors model.ValidProcessActions; unknown
// actions fall back to a title-cased form of the action string.
export function activityLabel(action: string, qualifier: string): string {
  switch (action) {
    case "submit_pr":
      return "Request submitted";
    case "update_pr":
      return "Request updated";
    case "reject_pr":
      return "Request rejected";
    case "add_pr_approver":
      return "Approver added";
    case "remove_pr_approver":
      return "Approver removed";
    case "pr_approval":
      return qualifier === "reject" ? "Approval rejected" : "Approved";
    case "rerequest_pr_approval":
      return "Approval re-requested";
    case "team_lead_approval":
      return qualifier === "reject" ? "Team lead rejected" : "Team lead approved";
    case "assign_pr":
      return qualifier === "unassign" ? "Unassigned" : "Assigned";
    case "update_pr_collaborators":
      return "Collaborators updated";
    case "update_pr_priority":
      return qualifier ? `Priority set to ${qualifier}` : "Priority updated";
    case "create_recommendation":
      return "Recommendation created";
    case "update_recommendation":
      return "Recommendation updated";
    case "delete_recommendation":
      return "Recommendation removed";
    case "rec_approval_legal":
      return qualifier === "revert" ? "Legal approval reverted" : "Legal approved";
    case "rec_approval_security":
      return qualifier === "revert" ? "Security approval reverted" : "Security approved";
    case "rec_approval_compliance":
      return qualifier === "revert" ? "Compliance approval reverted" : "Compliance approved";
    case "rec_approval_budget":
      if (qualifier === "reject") return "Budget rejected";
      if (qualifier === "revert") return "Budget approval reverted";
      return "Budget approved";
    case "request_rec_approval":
      return "Approval requested";
    case "remove_rec_approval":
      return "Approval card removed";
    case "update_budget_chain":
      return "Budget chain updated";
    case "assign_rec_legal":
      return qualifier === "unassign" ? "Legal assignee cleared" : "Legal assignee set";
    case "assign_rec_security":
      return qualifier === "unassign" ? "Security assignee cleared" : "Security assignee set";
    case "assign_rec_compliance":
      return qualifier === "unassign"
        ? "Compliance assignee cleared"
        : "Compliance assignee set";
    case "raise_rfi":
      return "RFI raised";
    case "clear_rfi":
      return "RFI cleared";
    case "add_quotation":
      return "Quotation added";
    case "update_quotation":
      return "Quotation updated";
    case "delete_quotation":
      return "Quotation removed";
    case "select_quotation":
      return "Quotation selected";
    case "add_draft_contract":
      return "Draft contract added";
    case "update_contract":
      return "Contract updated";
    case "delete_contract":
      return "Contract removed";
    case "sign_contract":
      return qualifier === "unsign" ? "Contract unsigned" : "Contract signed";
    case "create_grn":
      return "GRN recorded";
    case "update_grn":
      return "GRN updated";
    case "delete_grn":
      return "GRN removed";
    case "create_invoice":
      return "Invoice recorded";
    case "update_invoice":
      return "Invoice updated";
    case "delete_invoice":
      return "Invoice removed";
    case "invoice_status":
      return `Invoice ${qualifier || "updated"}`;
    default:
      return action.replace(/_/g, " ").replace(/^\w/, (c) => c.toUpperCase());
  }
}

// relativeTime renders an ISO timestamp as a compact "3h ago" style string.
export function relativeTime(iso: string): string {
  const then = new Date(iso).getTime();
  const secs = Math.round((Date.now() - then) / 1000);
  if (secs < 60) return "just now";
  const mins = Math.round(secs / 60);
  if (mins < 60) return `${mins}m ago`;
  const hrs = Math.round(mins / 60);
  if (hrs < 24) return `${hrs}h ago`;
  const days = Math.round(hrs / 24);
  if (days < 30) return `${days}d ago`;
  return new Date(iso).toLocaleDateString();
}
