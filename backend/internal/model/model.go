// Package model holds shared domain constants for the purchasing app.
package model

// Purchase request statuses.
const (
	StatusSubmitted        = "submitted"
	StatusUnderReview      = "under_review"
	StatusVendorSelected   = "vendor_selected"
	StatusContractPrepared = "contract_prepared"
	StatusOrderSigned      = "order_signed" // terminal for this phase; payment phase reserves StatusCompleted
	StatusCompleted        = "completed"
	StatusRejected         = "rejected"
	StatusCancelled        = "cancelled"
)

// Purchase-request approval statuses. Each named approver on a PR holds one of
// these; the requester re-requests a rejected approval to reset it to pending.
const (
	PRApprovalPending  = "pending"
	PRApprovalApproved = "approved"
	PRApprovalRejected = "rejected"
)

// IsApprovalDecision reports whether d is a terminal approver decision (the two
// outcomes an approver may record).
func IsApprovalDecision(d string) bool {
	return d == PRApprovalApproved || d == PRApprovalRejected
}

// Team lead approval statuses. Each PR carries a single team lead decision
// (reusing the same pending/approved/rejected vocabulary as named approvals).
const (
	TeamLeadPending  = "pending"
	TeamLeadApproved = "approved"
	TeamLeadRejected = "rejected"
)

// IsTeamLeadApproved reports whether a PR's team lead has approved it — the gate
// for procurement visibility and procurement actions.
func IsTeamLeadApproved(status string) bool {
	return status == TeamLeadApproved
}

// Purchase-request priority, set by procurement during triage (the assignment
// card). P3 is the default every PR is created with; the requester never picks
// one. Ordered highest-first, matching the DB CHECK on purchase_requests.priority.
const (
	PriorityP1 = "P1"
	PriorityP2 = "P2"
	PriorityP3 = "P3"
)

// PRPriorityDefault is the priority a PR carries until procurement changes it.
const PRPriorityDefault = PriorityP3

// ValidPRPriority reports whether s is a known priority.
func ValidPRPriority(s string) bool {
	return s == PriorityP1 || s == PriorityP2 || s == PriorityP3
}

// Quotation statuses.
const (
	QuoReceived        = "received"
	QuoUnderEvaluation = "under_evaluation"
	QuoSelected        = "selected"
	QuoRejected        = "rejected"
)

// Contract statuses.
const (
	ContractDraft           = "draft"
	ContractPendingApproval = "pending_approval"
	ContractApproved        = "approved"
	ContractRejected        = "rejected"
	ContractSigned          = "signed"
)

// Invoice statuses. An invoice walks received -> approved -> paid, and may be
// reverted one step each way (paid -> approved, approved -> received).
const (
	InvoiceReceived = "received"
	InvoiceApproved = "approved"
	InvoicePaid     = "paid"
)

// Invoice cost-allocation modes. An invoice is split across cost centers either
// entirely by percentage (values sum to 100) or entirely by absolute amount
// (values sum to the invoice total).
const (
	AllocByPercentage = "percentage"
	AllocByAmount     = "amount"
)

// ValidAllocationMode reports whether s is a known allocation mode.
func ValidAllocationMode(s string) bool {
	return s == AllocByPercentage || s == AllocByAmount
}

// ValidInvoiceTransition reports whether an invoice may move from one status to
// another. Forward moves advance one step; reverse moves undo one step.
func ValidInvoiceTransition(from, to string) bool {
	switch from {
	case InvoiceReceived:
		return to == InvoiceApproved
	case InvoiceApproved:
		return to == InvoiceReceived || to == InvoicePaid
	case InvoicePaid:
		return to == InvoiceApproved
	}
	return false
}

// Procurement recommendation approval types. Each is one required sign-off card
// on a PR's recommendation: budget owner (the named budget approver), legal,
// security and compliance. A recommendation requires a subset of these (budget is
// the default).
const (
	RecApprovalBudget     = "budget"
	RecApprovalLegal      = "legal"
	RecApprovalSecurity   = "security"
	RecApprovalCompliance = "compliance"
)

// RecApprovalTypes are all the approval card types a recommendation may require,
// in canonical (display) order.
var RecApprovalTypes = []string{RecApprovalBudget, RecApprovalLegal, RecApprovalSecurity, RecApprovalCompliance}

// RecTeamApprovalRoles maps every *team-backed* approval card to the role that
// designates its team: a member of that team may comment on and be assigned the
// card. The budget card is deliberately absent — its actor is the PR's named
// budget approver (an email match), not a role. Adding a team card (e.g.
// compliance) is a one-line change here plus a seeded `teams` row.
var RecTeamApprovalRoles = map[string]string{
	RecApprovalLegal:      RoleLegal,
	RecApprovalSecurity:   RoleSecurity,
	RecApprovalCompliance: RoleCompliance,
}

// RoleForRecApprovalType returns the team role backing a card type, and false for
// the budget card (which has no team, hence no assignee).
func RoleForRecApprovalType(t string) (string, bool) {
	role, ok := RecTeamApprovalRoles[t]
	return role, ok
}

// IsRecApprovalType reports whether t is a recognized recommendation approval type.
func IsRecApprovalType(t string) bool {
	if t == RecApprovalBudget {
		return true
	}
	_, ok := RecTeamApprovalRoles[t]
	return ok
}

// ConfigList describes one runtime-editable dropdown list managed from the
// Settings page (admin / procurement_admin). Key is the stored list_key; Label is
// the human title shown for the section.
type ConfigList struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// ConfigLists is the registry of known configurable dropdown lists. It drives
// server-side validation of list_key on create and the Settings-page section
// order/titles. The values themselves live in the config_options table.
var ConfigLists = []ConfigList{
	{Key: "entity", Label: "WSO2 entity"},
	{Key: "it_category", Label: "IT category"},
	{Key: "nonit_category", Label: "Non-IT category"},
	{Key: "engagement_type", Label: "Engagement type"},
	{Key: "currency", Label: "Currency"},
	{Key: "budget_category", Label: "Budget category"},
	{Key: "product", Label: "Product"},
	{Key: "region", Label: "Region"},
	{Key: "engagement_code", Label: "Engagement code"},
}

// IsConfigListKey reports whether key is a recognized configurable list.
func IsConfigListKey(key string) bool {
	for _, l := range ConfigLists {
		if l.Key == key {
			return true
		}
	}
	return false
}

// Document owner types (documents table is shared across procurement entities).
const (
	OwnerPurchaseRequest       = "purchase_request"
	OwnerQuotation             = "quotation"
	OwnerContract              = "contract"
	OwnerContractReview        = "contract_review"
	OwnerGRN                   = "grn"
	OwnerInvoice               = "invoice"
	OwnerRecommendationComment = "pr_recommendation_comment"
	OwnerRecommendationRFI     = "pr_recommendation_rfi"
	// OwnerQuotationExtraction holds a quotation PDF uploaded for extraction
	// *before* its quotation exists. On create the document is re-owned to the new
	// quotation and adopted into its initial-PDF slot, so this owner type is a
	// staging state rather than a resting place.
	OwnerQuotationExtraction = "quotation_extraction"
)

// Roles maintained within the app.
const (
	RoleStaff            = "staff"
	RoleProcurement      = "procurement"
	RoleProcurementAdmin = "procurement_admin"
	RoleAdmin            = "admin"
	RoleLegal            = "legal"
	RoleSecurity         = "security"
	RoleCompliance       = "compliance"
)

// AssignableRoles are the roles an admin may grant/revoke through user
// management. `staff` is a permanent baseline (auto-granted on first login and
// never removable), so it is not in this set.
var AssignableRoles = []string{RoleProcurement, RoleProcurementAdmin, RoleAdmin, RoleLegal, RoleSecurity, RoleCompliance}

// IsAssignableRole reports whether role can be added/removed via user management.
func IsAssignableRole(role string) bool {
	for _, r := range AssignableRoles {
		if r == role {
			return true
		}
	}
	return false
}

// EditableStatuses are the states in which a staff owner may modify a request
// (update details, add/remove documents).
var EditableStatuses = map[string]bool{
	StatusSubmitted:   true,
	StatusUnderReview: true,
}

// IsEditable reports whether a request in the given status can be edited by its owner.
func IsEditable(status string) bool {
	return EditableStatuses[status]
}
