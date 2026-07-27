package model

import "sort"

// Fixed action catalogs for the audit tables (process_events, audit_events).
//
// These constants are the single source of truth for the "fixed set of actions"
// — like BPMN task types, the set changes only when the process (code) changes.
// The repository validates every write against ValidProcessActions /
// ValidAuditActions, so an action string that is not declared here is rejected.
//
// The action names a *task*; the decision direction of that task (approve vs
// reject, sign vs unsign, the target invoice status, the contract source) is a
// separate qualifier value, not a distinct action.

// Process actions — human business-process tasks on a purchase request. Recorded
// in process_events. (PR status auto-advances are a side effect and are NOT
// recorded — they are derivable from this sequence.)
const (
	ProcessSubmitPR            = "submit_pr"
	ProcessUpdatePR            = "update_pr"
	ProcessRejectPR            = "reject_pr"
	ProcessAddApprover         = "add_pr_approver"
	ProcessRemoveApprover      = "remove_pr_approver"
	ProcessPRApproval          = "pr_approval" // qualifier: approve | reject
	ProcessRerequestPRApproval = "rerequest_pr_approval"
	ProcessTeamLeadApproval    = "team_lead_approval" // qualifier: approve | reject

	ProcessAssignPR              = "assign_pr" // qualifier: assign | unassign
	ProcessUpdatePRCollaborators = "update_pr_collaborators"

	ProcessCreateRecommendation = "create_recommendation"
	ProcessUpdateRecommendation = "update_recommendation"
	ProcessDeleteRecommendation = "delete_recommendation"
	ProcessRecApprovalLegal     = "rec_approval_legal"    // qualifier: approve | revert
	ProcessRecApprovalSecurity  = "rec_approval_security" // qualifier: approve | revert
	ProcessRecApprovalBudget    = "rec_approval_budget"   // qualifier: approve | reject | revert
	ProcessRequestRecApproval   = "request_rec_approval"  // qualifier: budget | legal | security
	ProcessRemoveRecApproval    = "remove_rec_approval"   // qualifier: budget | legal | security
	ProcessUpdateBudgetChain    = "update_budget_chain"   // qualifier: add | update | remove
	ProcessAssignRecLegal       = "assign_rec_legal"      // qualifier: assign | unassign
	ProcessAssignRecSecurity    = "assign_rec_security"   // qualifier: assign | unassign
	ProcessRaiseRFI             = "raise_rfi"
	ProcessClearRFI             = "clear_rfi"

	ProcessAddQuotation    = "add_quotation"
	ProcessUpdateQuotation = "update_quotation"
	ProcessDeleteQuotation = "delete_quotation"
	ProcessSelectQuotation = "select_quotation"

	ProcessAddDraftContract = "add_draft_contract" // qualifier: recommendation
	ProcessUpdateContract   = "update_contract"
	ProcessDeleteContract   = "delete_contract"
	ProcessSignContract     = "sign_contract" // qualifier: sign | unsign

	ProcessCreateGRN = "create_grn"
	ProcessUpdateGRN = "update_grn"
	ProcessDeleteGRN = "delete_grn"

	ProcessCreateInvoice = "create_invoice"
	ProcessUpdateInvoice = "update_invoice"
	ProcessDeleteInvoice = "delete_invoice"
	ProcessInvoiceStatus = "invoice_status" // qualifier: received | approved | paid
)

// Qualifier values (the decision dimension of a task). Not exhaustive constants
// for every action — several reuse existing model values (e.g. the invoice_status
// qualifier is InvoiceReceived/InvoiceApproved/InvoicePaid; grant_role uses the
// role name). These are the ones unique to the event layer.
const (
	QualifierApprove  = "approve"
	QualifierReject   = "reject"
	QualifierRevert   = "revert"
	QualifierSign     = "sign"
	QualifierUnsign   = "unsign"
	QualifierAssign   = "assign"
	QualifierUnassign = "unassign"

	// update_budget_chain
	QualifierAdd    = "add"
	QualifierUpdate = "update"
	QualifierRemove = "remove"

	// add_draft_contract source
	QualifierFromRecommendation = "recommendation"

	// set_user_active
	QualifierActive   = "active"
	QualifierInactive = "inactive"
)

// Audit actions — non-business-process (master-data / admin / config) mutations.
// Recorded in audit_events.
const (
	AuditCreateVendor = "create_vendor"
	AuditUpdateVendor = "update_vendor"

	AuditCreateBusinessUnit = "create_business_unit"
	AuditUpdateBusinessUnit = "update_business_unit"

	AuditCreateConfigOption = "create_config_option"
	AuditUpdateConfigOption = "update_config_option"
	AuditDeleteConfigOption = "delete_config_option"

	AuditCreateUser    = "create_user"
	AuditUpdateUser    = "update_user"     // edit an invited (not-yet-logged-in) user
	AuditGrantRole     = "grant_role"      // qualifier: role name
	AuditRevokeRole    = "revoke_role"     // qualifier: role name
	AuditSetUserActive = "set_user_active" // qualifier: active | inactive

	AuditConnectStorage   = "connect_storage"
	AuditSetStorageFolder = "set_storage_folder"
)

// Audit entity types.
const (
	EntityVendor       = "vendor"
	EntityBusinessUnit = "business_unit"
	EntityConfigOption = "config_option"
	EntityUser         = "user"
	EntityStorage      = "storage"
)

// ValidProcessActions / ValidAuditActions gate what the repository will write —
// enforcing the "fixed unless the code changes" guarantee.
var ValidProcessActions = setOf(
	ProcessSubmitPR, ProcessUpdatePR, ProcessRejectPR,
	ProcessAddApprover, ProcessRemoveApprover, ProcessPRApproval, ProcessRerequestPRApproval,
	ProcessTeamLeadApproval,
	ProcessAssignPR, ProcessUpdatePRCollaborators,
	ProcessCreateRecommendation, ProcessUpdateRecommendation, ProcessDeleteRecommendation,
	ProcessRecApprovalLegal, ProcessRecApprovalSecurity, ProcessRecApprovalBudget,
	ProcessRequestRecApproval, ProcessRemoveRecApproval,
	ProcessUpdateBudgetChain,
	ProcessAssignRecLegal, ProcessAssignRecSecurity,
	ProcessRaiseRFI, ProcessClearRFI,
	ProcessAddQuotation, ProcessUpdateQuotation, ProcessDeleteQuotation, ProcessSelectQuotation,
	ProcessAddDraftContract, ProcessUpdateContract, ProcessDeleteContract, ProcessSignContract,
	ProcessCreateGRN, ProcessUpdateGRN, ProcessDeleteGRN,
	ProcessCreateInvoice, ProcessUpdateInvoice, ProcessDeleteInvoice, ProcessInvoiceStatus,
)

var ValidAuditActions = setOf(
	AuditCreateVendor, AuditUpdateVendor,
	AuditCreateBusinessUnit, AuditUpdateBusinessUnit,
	AuditCreateConfigOption, AuditUpdateConfigOption, AuditDeleteConfigOption,
	AuditCreateUser, AuditUpdateUser, AuditGrantRole, AuditRevokeRole, AuditSetUserActive,
	AuditConnectStorage, AuditSetStorageFolder,
)

// RecApprovalAction maps a recommendation card type (budget/legal/security) to
// its fixed process action, so callers derive the action from the URL {type}.
func RecApprovalAction(approvalType string) (string, bool) {
	switch approvalType {
	case RecApprovalLegal:
		return ProcessRecApprovalLegal, true
	case RecApprovalSecurity:
		return ProcessRecApprovalSecurity, true
	case RecApprovalBudget:
		return ProcessRecApprovalBudget, true
	}
	return "", false
}

// RecAssignAction maps a legal/security card type to its assignment process
// action. The budget card has no assignee, so it returns ("", false).
func RecAssignAction(approvalType string) (string, bool) {
	switch approvalType {
	case RecApprovalLegal:
		return ProcessAssignRecLegal, true
	case RecApprovalSecurity:
		return ProcessAssignRecSecurity, true
	}
	return "", false
}

func setOf(vals ...string) map[string]bool {
	m := make(map[string]bool, len(vals))
	for _, v := range vals {
		m[v] = true
	}
	return m
}

// SortedProcessActions / SortedAuditActions return the fixed action catalogs as
// sorted slices, for the events-view action filter dropdowns (the maps have no
// stable order).
func SortedProcessActions() []string { return sortedKeys(ValidProcessActions) }
func SortedAuditActions() []string   { return sortedKeys(ValidAuditActions) }

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
