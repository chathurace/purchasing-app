package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/cs/purchasing-app/internal/email"
	"github.com/cs/purchasing-app/internal/middleware"
	"github.com/cs/purchasing-app/internal/model"
	"github.com/cs/purchasing-app/internal/repository"
	"github.com/cs/purchasing-app/internal/storage"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"
)

// maxUploadBytes caps a single document upload (25 MiB).
const maxUploadBytes = 25 << 20

// allowedExtensions are the document types staff may upload.
var allowedExtensions = map[string]bool{".pdf": true, ".docx": true}

type PurchaseRequestsHandler struct {
	Repo       *repository.Repository
	Storage    storage.Store
	Mailer     email.Mailer
	AppBaseURL string
	Log        zerolog.Logger
}

// --- request/response payloads ---

type itemInput struct {
	Description string  `json:"description"`
	Quantity    float64 `json:"quantity"`
}

type linkInput struct {
	URL   string `json:"url"`
	Label string `json:"label"`
}

type prInput struct {
	Title        string      `json:"title"`
	BudgetUnitID *int64      `json:"budget_unit_id"`
	Comments     string      `json:"comments"`
	Items        []itemInput `json:"items"`
	Links        []linkInput `json:"links"`
	ApproverIDs  []int64     `json:"approver_ids"`
	// Requisition form fields.
	Team                string          `json:"team"`
	Entity              string          `json:"entity"`
	Category            string          `json:"category"`
	EstimatedValue      float64         `json:"estimated_value"`
	Currency            string          `json:"currency"`
	BudgetApproverName  string          `json:"budget_approver_name"`
	BudgetApproverEmail string          `json:"budget_approver_email"`
	Details             json.RawMessage `json:"details"`
	TeamLeadEmail       string          `json:"team_lead_email"`
}

func (in prInput) toRepo() repository.PurchaseRequestInput {
	out := repository.PurchaseRequestInput{
		Title:               strings.TrimSpace(in.Title),
		BudgetUnitID:        in.BudgetUnitID,
		Comments:            in.Comments,
		Team:                strings.TrimSpace(in.Team),
		Entity:              strings.TrimSpace(in.Entity),
		Category:            strings.TrimSpace(in.Category),
		EstimatedValue:      in.EstimatedValue,
		Currency:            strings.TrimSpace(in.Currency),
		BudgetApproverName:  strings.TrimSpace(in.BudgetApproverName),
		BudgetApproverEmail: strings.TrimSpace(in.BudgetApproverEmail),
		Details:             in.Details,
		TeamLeadEmail:       strings.TrimSpace(in.TeamLeadEmail),
	}
	for _, it := range in.Items {
		desc := strings.TrimSpace(it.Description)
		if desc == "" {
			continue
		}
		out.Items = append(out.Items, repository.Item{Description: desc, Quantity: it.Quantity})
	}
	for _, ln := range in.Links {
		u := strings.TrimSpace(ln.URL)
		if u == "" {
			continue
		}
		out.Links = append(out.Links, repository.Link{URL: u, Label: strings.TrimSpace(ln.Label)})
	}
	return out
}

// --- access helpers ---

// callerCanView reports whether the caller may view the request. Until the team
// lead has approved it, a PR is visible only to the requester, the named team
// lead (email match), and admins. Once team-lead-approved, the broader audience
// applies: procurement/procurement_admin/admin, a named approver, or an actor on the
// procurement recommendation's approval cards (legal/security/budget).
func (h *PurchaseRequestsHandler) callerCanView(r *http.Request, pr *repository.PurchaseRequest) bool {
	user := middleware.UserFromCtx(r.Context())
	if user != nil && user.ID == pr.RequesterID {
		return true
	}
	if middleware.HasRole(r.Context(), model.RoleAdmin) {
		return true
	}
	if user != nil && h.callerIsTeamLead(user, pr) {
		return true
	}
	// Everyone else must wait for team-lead approval.
	if !model.IsTeamLeadApproved(pr.TeamLeadStatus) {
		return false
	}
	if middleware.HasProcurementAccess(r.Context()) {
		return true
	}
	if user != nil {
		for _, a := range pr.Approvals {
			if a.ApproverID == user.ID {
				return true
			}
		}
	}
	if pr.Recommendation != nil {
		for _, a := range pr.Recommendation.Approvals {
			if ok, _ := h.canActOnRecType(r, pr, a.ApprovalType); ok {
				return true
			}
		}
	}
	return false
}

// callerIsTeamLead reports whether the user is the PR's named team lead — a
// case-insensitive email match. Emails are stored lowercased; we normalize the
// caller's side too for safety.
func (h *PurchaseRequestsHandler) callerIsTeamLead(user *repository.User, pr *repository.PurchaseRequest) bool {
	if user == nil || pr.TeamLeadEmail == "" {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(user.Email), pr.TeamLeadEmail)
}

// canActOnRecType reports whether the caller has a stake in a recommendation
// approval card — the predicate for viewing the PR, for *commenting* and (for
// the budget card) for approving: legal/security require the matching role
// (admin passes); the budget card requires being a qualified budget approver of
// the PR's budget unit (or admin) — any approver of the bracket the
// recommendation's estimated value + currency resolve to. Approving a
// legal/security card additionally requires being its assignee — see
// canApproveRecType.
func (h *PurchaseRequestsHandler) canActOnRecType(r *http.Request, pr *repository.PurchaseRequest, approvalType string) (bool, error) {
	ctx := r.Context()
	switch approvalType {
	case model.RecApprovalLegal:
		return middleware.HasRole(ctx, model.RoleLegal), nil
	case model.RecApprovalSecurity:
		return middleware.HasRole(ctx, model.RoleSecurity), nil
	case model.RecApprovalBudget:
		if middleware.HasRole(ctx, model.RoleAdmin) {
			return true, nil
		}
		user := middleware.UserFromCtx(ctx)
		if user == nil {
			return false, nil
		}
		ok, err := h.Repo.IsBudgetApproverForPR(ctx, pr.ID, user.ID)
		if err != nil {
			zerolog.Ctx(ctx).Error().Err(err).Int64("pr_id", pr.ID).Msg("check budget approver")
		}
		return ok, err
	}
	return false, nil
}

// canApproveRecType reports whether the caller may toggle a card's approval. For
// legal/security only the card's *assignee* may approve (a team member assigns the
// card to themselves first); the budget card keeps the canActOnRecType rule.
func (h *PurchaseRequestsHandler) canApproveRecType(r *http.Request, pr *repository.PurchaseRequest, a *repository.RecApproval) (bool, error) {
	switch a.ApprovalType {
	case model.RecApprovalLegal, model.RecApprovalSecurity:
		user := middleware.UserFromCtx(r.Context())
		return user != nil && a.AssigneeID != nil && *a.AssigneeID == user.ID, nil
	default:
		return h.canActOnRecType(r, pr, a.ApprovalType)
	}
}

// canAssignRecType reports whether the caller may set the assignee of a
// legal/security card: any member of that team, or any procurement user (procurement /
// procurement_admin / admin). The budget card has no assignee.
func (h *PurchaseRequestsHandler) canAssignRecType(ctx context.Context, approvalType string) bool {
	switch approvalType {
	case model.RecApprovalLegal:
		return middleware.HasRole(ctx, model.RoleLegal) || middleware.HasProcurementAccess(ctx)
	case model.RecApprovalSecurity:
		return middleware.HasRole(ctx, model.RoleSecurity) || middleware.HasProcurementAccess(ctx)
	}
	return false
}

// attachRecActionable fills each approval card's per-caller capability flags
// (comment / approve / assign) and the recommendation's my_actionable_types (the
// cards the caller may comment on), so the UI shows controls only where allowed.
// Safe to call when the PR has no recommendation.
func (h *PurchaseRequestsHandler) attachRecActionable(r *http.Request, pr *repository.PurchaseRequest) {
	if pr == nil || pr.Recommendation == nil {
		return
	}
	actionable := []string{}
	for i := range pr.Recommendation.Approvals {
		a := &pr.Recommendation.Approvals[i]
		if ok, _ := h.canActOnRecType(r, pr, a.ApprovalType); ok {
			a.CanComment = true
			actionable = append(actionable, a.ApprovalType)
		}
		if ok, _ := h.canApproveRecType(r, pr, a); ok {
			a.CanApprove = true
		}
		a.CanAssign = h.canAssignRecType(r.Context(), a.ApprovalType)
	}
	pr.Recommendation.MyActionableTypes = actionable
}

// attachTeamLeadActionable sets my_team_lead_actionable for the caller (true when
// they are the named team lead or an admin), so the UI shows the decision
// controls only where allowed.
func (h *PurchaseRequestsHandler) attachTeamLeadActionable(r *http.Request, pr *repository.PurchaseRequest) {
	if pr == nil {
		return
	}
	user := middleware.UserFromCtx(r.Context())
	pr.MyTeamLeadActionable = h.callerIsTeamLead(user, pr) || middleware.HasRole(r.Context(), model.RoleAdmin)
}

// callerCanEdit reports whether the caller may modify the request: the owner
// while it is in an editable state, or an admin (forward-compat).
func (h *PurchaseRequestsHandler) callerCanEdit(r *http.Request, pr *repository.PurchaseRequest) bool {
	if middleware.HasRole(r.Context(), model.RoleAdmin) {
		return true
	}
	user := middleware.UserFromCtx(r.Context())
	return user != nil && user.ID == pr.RequesterID && model.IsEditable(pr.Status)
}

// --- handlers ---

// List returns all requests for procurement*/admin, or — for other users — the
// requests they own, have been asked to approve, or carry a procurement
// recommendation card they can act on (legal/security/budget owner).
func (h *PurchaseRequestsHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	seesAll := middleware.HasProcurementAccess(ctx)
	user := middleware.UserFromCtx(ctx)

	// ?scope=mine  -> only PRs the caller submitted (Requests tab for staff/approvers)
	// ?scope=approvals -> only PRs awaiting the caller's decision (Approvals tab)
	// (absent) -> legacy behavior: seesAll for procurement, union otherwise.
	var scope repository.PRListScope
	switch r.URL.Query().Get("scope") {
	case "mine":
		scope = repository.PRScopeMine
	case "approvals":
		scope = repository.PRScopeApprovals
	}

	prs, err := h.Repo.ListPurchaseRequests(ctx, user.ID, user.Email, seesAll,
		middleware.HasRole(ctx, model.RoleAdmin),
		middleware.HasRole(ctx, model.RoleLegal), middleware.HasRole(ctx, model.RoleSecurity), scope)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("list purchase requests")
		writeError(w, http.StatusInternalServerError, "failed to list requests")
		return
	}
	if prs == nil {
		prs = []*repository.PurchaseRequest{}
	}
	writeJSON(w, http.StatusOK, prs)
}

func (h *PurchaseRequestsHandler) Create(w http.ResponseWriter, r *http.Request) {
	var in prInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	user := middleware.UserFromCtx(r.Context())

	repoIn := in.toRepo()
	// Team lead is mandatory: the PR is gated behind their approval. It cannot be
	// the requester themselves (nobody approves their own request).
	tlEmail := strings.ToLower(strings.TrimSpace(in.TeamLeadEmail))
	if tlEmail == "" {
		writeError(w, http.StatusBadRequest, "a team lead email is required")
		return
	}
	if tlEmail == strings.ToLower(strings.TrimSpace(user.Email)) {
		writeError(w, http.StatusBadRequest, "you cannot list yourself as the team lead")
		return
	}
	// Approvers are optional now — procurement sign-off is handled by the
	// recommendation, not a PR-level approver gate. Validate only any provided.
	if len(in.ApproverIDs) > 0 {
		approverIDs, errMsg := h.validateApprovers(r.Context(), user.ID, in.ApproverIDs)
		if errMsg != "" {
			writeError(w, http.StatusBadRequest, errMsg)
			return
		}
		repoIn.ApproverIDs = approverIDs
	}
	pr, err := h.Repo.CreatePurchaseRequest(r.Context(), user.ID, repoIn)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("create purchase request")
		writeError(w, http.StatusInternalServerError, "failed to create request")
		return
	}
	recordProcessEvent(r, h.Repo, pr.ID, model.ProcessSubmitPR, "")
	h.notifyTeamLeadRequested(pr, user)
	h.notifyPRSubmitted(r, pr)
	if len(pr.Approvals) > 0 {
		h.notifyApprovalRequested(pr, pr.Approvals, user)
	}
	writeJSON(w, http.StatusCreated, pr)
}

// validateApprovers normalizes the requested approver IDs: deduped, with the
// requester removed, and every id confirmed to be an active user. It returns the
// cleaned list, or a non-empty user-facing error message on failure.
func (h *PurchaseRequestsHandler) validateApprovers(ctx context.Context, requesterID int64, ids []int64) ([]int64, string) {
	active, err := h.Repo.ListActiveUsers(ctx)
	if err != nil {
		zerolog.Ctx(ctx).Error().Err(err).Msg("validate approvers: list users")
		return nil, "failed to validate approvers"
	}
	activeSet := make(map[int64]bool, len(active))
	for _, u := range active {
		activeSet[u.ID] = true
	}
	seen := map[int64]bool{}
	out := []int64{}
	for _, id := range ids {
		if id == requesterID {
			return nil, "you cannot list yourself as an approver"
		}
		if seen[id] {
			continue
		}
		if !activeSet[id] {
			return nil, "one or more selected approvers are not valid active users"
		}
		seen[id] = true
		out = append(out, id)
	}
	if len(out) == 0 {
		return nil, "select at least one approver"
	}
	return out, ""
}

func (h *PurchaseRequestsHandler) Get(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.load(w, r)
	if !ok {
		return
	}
	if !h.callerCanView(r, pr) {
		writeError(w, http.StatusForbidden, "not allowed to view this request")
		return
	}
	h.attachRecActionable(r, pr)
	h.attachTeamLeadActionable(r, pr)
	h.attachAssignmentActionable(r, pr)
	writeJSON(w, http.StatusOK, pr)
}

// relatedDocuments bundles the procurement records of a single case (one PR and
// its quotations and contracts) so a detail page can show all the records
// associated with whatever entity is being viewed.
type relatedDocuments struct {
	PurchaseRequest prSummary               `json:"purchase_request"`
	Quotations      []*repository.Quotation `json:"quotations"`
	Contracts       []*repository.Contract  `json:"contracts"`
	GRNs            []*repository.GRN       `json:"grns"`
	Invoices        []*repository.Invoice   `json:"invoices"`
}

type prSummary struct {
	ID        int64   `json:"id"`
	Reference *string `json:"reference"`
	Title     string  `json:"title"`
	Status    string  `json:"status"`
}

// Related returns every procurement record linked to the case anchored on this
// purchase request. Gated to procurement access so it never widens the read scope of
// the per-entity list endpoints it draws from.
func (h *PurchaseRequestsHandler) Related(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasProcurementAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "procurement access required")
		return
	}
	pr, ok := h.load(w, r)
	if !ok {
		return
	}
	quotations, err := h.Repo.ListQuotationsForPR(r.Context(), pr.ID)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("related: list quotations")
		writeError(w, http.StatusInternalServerError, "failed to load related documents")
		return
	}
	contracts, err := h.Repo.ListContracts(r.Context(), &pr.ID)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("related: list contracts")
		writeError(w, http.StatusInternalServerError, "failed to load related documents")
		return
	}
	grns, err := h.Repo.ListGRNsForPR(r.Context(), pr.ID)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("related: list grns")
		writeError(w, http.StatusInternalServerError, "failed to load related documents")
		return
	}
	invoices, err := h.Repo.ListInvoicesForPR(r.Context(), pr.ID)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("related: list invoices")
		writeError(w, http.StatusInternalServerError, "failed to load related documents")
		return
	}
	writeJSON(w, http.StatusOK, relatedDocuments{
		PurchaseRequest: prSummary{ID: pr.ID, Reference: pr.Reference, Title: pr.Title, Status: pr.Status},
		Quotations:      quotations,
		Contracts:       contracts,
		GRNs:            grns,
		Invoices:        invoices,
	})
}

func (h *PurchaseRequestsHandler) Update(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.load(w, r)
	if !ok {
		return
	}
	if !h.callerCanEdit(r, pr) {
		writeError(w, http.StatusForbidden, "this request can no longer be edited")
		return
	}
	var in prInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.Repo.UpdatePurchaseRequest(r.Context(), pr.ID, in.toRepo()); err != nil {
		reqLog(r).Error().Err(err).Msg("update purchase request")
		writeError(w, http.StatusInternalServerError, "failed to update request")
		return
	}
	recordProcessEvent(r, h.Repo, pr.ID, model.ProcessUpdatePR, "")
	updated, err := h.Repo.GetPurchaseRequest(r.Context(), pr.ID)
	if err != nil {
		reqLog(r).Error().Err(err).Int64("pr_id", pr.ID).Msg("reload purchase request after mutation")
		writeError(w, http.StatusInternalServerError, "failed to reload request")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// Reject marks a purchase request as rejected with a required comment and
// cancels its open quotations/unsigned contracts. Procurement access only.
func (h *PurchaseRequestsHandler) Reject(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasProcurementAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "procurement access required")
		return
	}
	pr, ok := h.load(w, r)
	if !ok {
		return
	}
	if pr.Status == model.StatusRejected || pr.Status == model.StatusCancelled ||
		pr.Status == model.StatusOrderSigned || pr.Status == model.StatusCompleted {
		writeError(w, http.StatusConflict, "request can no longer be rejected")
		return
	}
	var in struct {
		Comment string `json:"comment"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(in.Comment) == "" {
		writeError(w, http.StatusBadRequest, "a rejection comment is required")
		return
	}
	user := middleware.UserFromCtx(r.Context())
	if err := h.Repo.RejectPurchaseRequest(r.Context(), pr.ID, strings.TrimSpace(in.Comment), user.ID); err != nil {
		reqLog(r).Error().Err(err).Msg("reject purchase request")
		writeError(w, http.StatusInternalServerError, "failed to reject request")
		return
	}
	recordProcessEvent(r, h.Repo, pr.ID, model.ProcessRejectPR, "")
	updated, err := h.Repo.GetPurchaseRequest(r.Context(), pr.ID)
	if err != nil {
		reqLog(r).Error().Err(err).Int64("pr_id", pr.ID).Msg("reload purchase request after mutation")
		writeError(w, http.StatusInternalServerError, "failed to reload request")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// --- approvals ---

// callerCanManageApprovers reports whether the caller may add/remove approvers or
// re-request a rejected approval: the owner while the request is editable, or an
// admin. Mirrors callerCanEdit.
func (h *PurchaseRequestsHandler) callerCanManageApprovers(r *http.Request, pr *repository.PurchaseRequest) bool {
	if middleware.HasRole(r.Context(), model.RoleAdmin) {
		return true
	}
	user := middleware.UserFromCtx(r.Context())
	return user != nil && user.ID == pr.RequesterID && model.IsEditable(pr.Status)
}

// AddApprover adds an approver to the request (owner, while editable).
func (h *PurchaseRequestsHandler) AddApprover(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.load(w, r)
	if !ok {
		return
	}
	if !h.callerCanManageApprovers(r, pr) {
		writeError(w, http.StatusForbidden, "approvers can only be changed while the request is editable")
		return
	}
	var in struct {
		ApproverID int64 `json:"approver_id"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ids, errMsg := h.validateApprovers(r.Context(), pr.RequesterID, []int64{in.ApproverID})
	if errMsg != "" {
		writeError(w, http.StatusBadRequest, errMsg)
		return
	}
	approval, err := h.Repo.AddApprover(r.Context(), pr.ID, ids[0])
	if err != nil {
		reqLog(r).Error().Err(err).Msg("add approver")
		writeError(w, http.StatusInternalServerError, "failed to add approver")
		return
	}
	recordProcessEvent(r, h.Repo, pr.ID, model.ProcessAddApprover, "")
	requester := middleware.UserFromCtx(r.Context())
	if approval.Status == model.PRApprovalPending {
		h.notifyApprovalRequested(pr, []repository.PRApproval{*approval}, requester)
	}
	h.reloadPR(w, r, pr.ID)
}

// RemoveApprover removes an approver from the request (owner, while editable).
// The last remaining approver cannot be removed.
func (h *PurchaseRequestsHandler) RemoveApprover(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.load(w, r)
	if !ok {
		return
	}
	if !h.callerCanManageApprovers(r, pr) {
		writeError(w, http.StatusForbidden, "approvers can only be changed while the request is editable")
		return
	}
	approverID, err := parseID(r, "approverID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid approver id")
		return
	}
	if err := h.Repo.RemoveApprover(r.Context(), pr.ID, approverID); err != nil {
		if errors.Is(err, repository.ErrInvalidState) {
			writeError(w, http.StatusConflict, "a request must keep at least one approver")
			return
		}
		reqLog(r).Error().Err(err).Msg("remove approver")
		writeError(w, http.StatusInternalServerError, "failed to remove approver")
		return
	}
	recordProcessEvent(r, h.Repo, pr.ID, model.ProcessRemoveApprover, "")
	h.reloadPR(w, r, pr.ID)
}

// RequestApprovalAgain resets a rejected approval to pending so the approver can
// reconsider (owner, while editable).
func (h *PurchaseRequestsHandler) RequestApprovalAgain(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.load(w, r)
	if !ok {
		return
	}
	if !h.callerCanManageApprovers(r, pr) {
		writeError(w, http.StatusForbidden, "approvals can only be re-requested while the request is editable")
		return
	}
	approverID, err := parseID(r, "approverID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid approver id")
		return
	}
	if err := h.Repo.RequestApprovalAgain(r.Context(), pr.ID, approverID); err != nil {
		if errors.Is(err, repository.ErrInvalidState) {
			writeError(w, http.StatusConflict, "only a rejected approval can be re-requested")
			return
		}
		reqLog(r).Error().Err(err).Msg("re-request approval")
		writeError(w, http.StatusInternalServerError, "failed to re-request approval")
		return
	}
	recordProcessEvent(r, h.Repo, pr.ID, model.ProcessRerequestPRApproval, "")
	requester := middleware.UserFromCtx(r.Context())
	if approval, err := h.Repo.GetApproval(r.Context(), pr.ID, approverID); err == nil {
		h.notifyApprovalRequested(pr, []repository.PRApproval{*approval}, requester)
	}
	h.reloadPR(w, r, pr.ID)
}

// RecordApprovalDecision records the calling approver's own decision (approve or
// reject) with a comment. Only a named approver on the request may act, and only
// on their own row.
func (h *PurchaseRequestsHandler) RecordApprovalDecision(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.load(w, r)
	if !ok {
		return
	}
	user := middleware.UserFromCtx(r.Context())
	isApprover, err := h.Repo.IsApprover(r.Context(), pr.ID, user.ID)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("check approver")
		writeError(w, http.StatusInternalServerError, "failed to record decision")
		return
	}
	if !isApprover {
		writeError(w, http.StatusForbidden, "only a named approver can act on this request")
		return
	}
	if pr.Status == model.StatusRejected || pr.Status == model.StatusCancelled {
		writeError(w, http.StatusConflict, "this request is closed")
		return
	}
	var in struct {
		Decision string `json:"decision"` // approve | reject
		Comment  string `json:"comment"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	var status string
	switch in.Decision {
	case "approve":
		status = model.PRApprovalApproved
	case "reject":
		status = model.PRApprovalRejected
	default:
		writeError(w, http.StatusBadRequest, "decision must be approve or reject")
		return
	}
	comment := strings.TrimSpace(in.Comment)
	if status == model.PRApprovalRejected && comment == "" {
		writeError(w, http.StatusBadRequest, "a comment is required when rejecting")
		return
	}
	if err := h.Repo.RecordApprovalDecision(r.Context(), pr.ID, user.ID, status, comment); err != nil {
		if errors.Is(err, repository.ErrInvalidState) {
			writeError(w, http.StatusForbidden, "only a named approver can act on this request")
			return
		}
		reqLog(r).Error().Err(err).Msg("record approval decision")
		writeError(w, http.StatusInternalServerError, "failed to record decision")
		return
	}
	recordProcessEvent(r, h.Repo, pr.ID, model.ProcessPRApproval, in.Decision) // in.Decision: approve | reject
	h.notifyDecision(pr, user, status, comment)
	h.reloadPR(w, r, pr.ID)
}

// TeamLeadDecision records the team lead's approve/reject decision (with notes)
// on a PR — the gate that lets procurement see and act on it. Only the named team
// lead (email match) or an admin may act; the decision may be revised at any
// time ("edit decision"). Notes are required when rejecting.
func (h *PurchaseRequestsHandler) TeamLeadDecision(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.load(w, r)
	if !ok {
		return
	}
	user := middleware.UserFromCtx(r.Context())
	if !h.callerIsTeamLead(user, pr) && !middleware.HasRole(r.Context(), model.RoleAdmin) {
		writeError(w, http.StatusForbidden, "only the team lead can act on this request")
		return
	}
	var in struct {
		Decision string `json:"decision"` // approve | reject | pending
		Notes    string `json:"notes"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	// "pending" reopens a prior decision (edit decision → move to pending): it
	// clears the decision without notes or a requester notification.
	if in.Decision == "pending" {
		if err := h.Repo.ResetTeamLeadToPending(r.Context(), pr.ID); err != nil {
			reqLog(r).Error().Err(err).Msg("reset team lead to pending")
			writeError(w, http.StatusInternalServerError, "failed to reopen decision")
			return
		}
		recordProcessEvent(r, h.Repo, pr.ID, model.ProcessTeamLeadApproval, "pending")
		h.reloadPR(w, r, pr.ID)
		return
	}
	var status string
	switch in.Decision {
	case "approve":
		status = model.TeamLeadApproved
	case "reject":
		status = model.TeamLeadRejected
	default:
		writeError(w, http.StatusBadRequest, "decision must be approve, reject or pending")
		return
	}
	notes := strings.TrimSpace(in.Notes)
	if status == model.TeamLeadRejected && notes == "" {
		writeError(w, http.StatusBadRequest, "notes are required when rejecting")
		return
	}
	if err := h.Repo.RecordTeamLeadDecision(r.Context(), pr.ID, user.ID, status, notes); err != nil {
		reqLog(r).Error().Err(err).Msg("record team lead decision")
		writeError(w, http.StatusInternalServerError, "failed to record decision")
		return
	}
	recordProcessEvent(r, h.Repo, pr.ID, model.ProcessTeamLeadApproval, in.Decision) // in.Decision: approve | reject
	h.notifyDecision(pr, user, status, notes)
	if status == model.TeamLeadApproved {
		h.notifyProcurementTeamOfApproval(r, pr, user)
	} else {
		h.notifyTeamLeadRejected(r, pr, notes)
	}
	h.reloadPR(w, r, pr.ID)
}

// RemindTeamLead re-sends the pending-approval notification to the PR's team lead.
// Allowed to the requester, procurement_admin or admin, while approval is pending.
func (h *PurchaseRequestsHandler) RemindTeamLead(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.load(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	user := middleware.UserFromCtx(ctx)
	isRequester := user != nil && user.ID == pr.RequesterID
	if !isRequester && !middleware.HasRole(ctx, model.RoleProcurementAdmin) && !middleware.HasRole(ctx, model.RoleAdmin) {
		writeError(w, http.StatusForbidden, "only the requester, procurement_admin or admin can send a reminder")
		return
	}
	if pr.TeamLeadStatus != model.TeamLeadPending {
		writeError(w, http.StatusConflict, "the team lead has already decided this request")
		return
	}
	if pr.TeamLeadEmail == "" {
		writeError(w, http.StatusBadRequest, "this request has no team lead")
		return
	}
	h.notifyTeamLeadReminder(pr)
	w.WriteHeader(http.StatusNoContent)
}

// UpdateTeamLeadEmail changes the PR's named team lead (by email) while approval
// is still pending. Allowed to the requester, procurement_admin or admin. The new team
// lead is notified. It cannot be the requester themselves.
func (h *PurchaseRequestsHandler) UpdateTeamLeadEmail(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.load(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	user := middleware.UserFromCtx(ctx)
	isRequester := user != nil && user.ID == pr.RequesterID
	if !isRequester && !middleware.HasRole(ctx, model.RoleProcurementAdmin) && !middleware.HasRole(ctx, model.RoleAdmin) {
		writeError(w, http.StatusForbidden, "only the requester, procurement_admin or admin can change the team lead")
		return
	}
	if pr.TeamLeadStatus != model.TeamLeadPending {
		writeError(w, http.StatusConflict, "the team lead can only be changed while approval is pending")
		return
	}
	var in struct {
		TeamLeadEmail string `json:"team_lead_email"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.TeamLeadEmail))
	if email == "" {
		writeError(w, http.StatusBadRequest, "a team lead email is required")
		return
	}
	if pr.Requester != nil && strings.EqualFold(email, strings.TrimSpace(pr.Requester.Email)) {
		writeError(w, http.StatusBadRequest, "the team lead cannot be the requester")
		return
	}
	if err := h.Repo.UpdateTeamLeadEmail(ctx, pr.ID, email); err != nil {
		reqLog(r).Error().Err(err).Msg("update team lead email")
		writeError(w, http.StatusInternalServerError, "failed to update team lead")
		return
	}
	recordProcessEvent(r, h.Repo, pr.ID, model.ProcessUpdatePR, "team_lead_email")
	// Notify the new team lead. Use the PR's requester as the "who" when available.
	pr.TeamLeadEmail = email
	requester := user
	if pr.Requester != nil {
		requester = &repository.User{Name: pr.Requester.Name, Email: pr.Requester.Email}
	}
	h.notifyTeamLeadRequested(pr, requester)
	h.reloadPR(w, r, pr.ID)
}

// reloadPR re-reads the request and writes it as the response (used by the
// approval mutations so the client gets the fresh approval state).
// SetBudgetApprover updates only the PR's named budget approver (name + email).
// Procurement only — used from the recommendation's budget card to reconcile the
// named approver against the designated one; it does not touch approval state
// and works regardless of the PR's editable status.
func (h *PurchaseRequestsHandler) SetBudgetApprover(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasProcurementAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "procurement access required")
		return
	}
	pr, ok := h.load(w, r)
	if !ok {
		return
	}
	var in struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.Repo.SetBudgetApprover(r.Context(), pr.ID, strings.TrimSpace(in.Name), strings.TrimSpace(in.Email)); err != nil {
		reqLog(r).Error().Err(err).Msg("set budget approver")
		writeError(w, http.StatusInternalServerError, "failed to update budget approver")
		return
	}
	ensureCollaborator(r, h.Repo, pr.ID)
	h.reloadPR(w, r, pr.ID)
}

func (h *PurchaseRequestsHandler) reloadPR(w http.ResponseWriter, r *http.Request, id int64) {
	pr, err := h.Repo.GetPurchaseRequest(r.Context(), id)
	if err != nil {
		reqLog(r).Error().Err(err).Int64("pr_id", id).Msg("reload purchase request")
		writeError(w, http.StatusInternalServerError, "failed to reload request")
		return
	}
	h.attachRecActionable(r, pr)
	h.attachTeamLeadActionable(r, pr)
	h.attachAssignmentActionable(r, pr)
	writeJSON(w, http.StatusOK, pr)
}

// --- approval email notifications (best-effort) ---

// prDisplayRef returns a PR's human-readable reference, falling back to the
// system id (#42) when none has been assigned. The reference is a display label
// whose format may change over time, kept separate from the immutable id.
func prDisplayRef(pr *repository.PurchaseRequest) string {
	if pr.Reference != nil && *pr.Reference != "" {
		return *pr.Reference
	}
	return fmt.Sprintf("#%d", pr.ID)
}

func (h *PurchaseRequestsHandler) prURL(pr *repository.PurchaseRequest) string {
	return prURLWith(h.AppBaseURL, pr)
}

// prURLWith renders a PR reference optionally followed by a deep link, from a bare
// base URL (so callers without a *PurchaseRequestsHandler can build the same link).
func prURLWith(appBaseURL string, pr *repository.PurchaseRequest) string {
	ref := prDisplayRef(pr)
	if appBaseURL == "" {
		return ref
	}
	return fmt.Sprintf("%s (%s/requests/%d)", ref, strings.TrimRight(appBaseURL, "/"), pr.ID)
}

// sendAsync fires a single email on a background goroutine so SMTP latency never
// blocks the HTTP response. Failures are logged, never surfaced to the user.
func (h *PurchaseRequestsHandler) sendAsync(to, subject, body string) {
	sendMailAsync(h.Mailer, h.Log, to, nil, subject, body)
}

// sendMailAsync sends one email (with optional Cc) on a background goroutine — the
// package-level primitive behind sendAsync/sendAsyncCC and notifyPRActivity, so
// handlers without a *PurchaseRequestsHandler (e.g. QuotationsHandler) can use it.
// A nil mailer or blank primary recipient is a no-op; failures are logged.
func sendMailAsync(mailer email.Mailer, log zerolog.Logger, to string, cc []string, subject, body string) {
	if mailer == nil || strings.TrimSpace(to) == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := mailer.SendCC(ctx, to, cc, subject, body); err != nil {
			// Detached goroutine (outlives the request): use the base logger,
			// not the request-scoped one, to avoid racing the access-log read.
			log.Warn().Err(err).Str("to", to).Msg("notification email send failed")
		}
	}()
}

// notifyPRActivity emails the Procurement team's shared mailbox and the PR's assignee
// + collaborators about activity on the PR. The team email is the primary recipient
// with the others Cc'd; when no team email is configured the first assignee/collaborator
// becomes the primary recipient instead. excludeEmail (the acting user) is dropped from
// the assignee/collaborator set to avoid self-notifying. Best-effort — a nil mailer or a
// lookup failure just means no mail is sent. pr must be loaded via GetPurchaseRequest so
// Assignee and Collaborators are populated (they are empty before assignment, e.g. at
// submit / team-lead stage, which naturally reduces this to a team-only notice).
func notifyPRActivity(ctx context.Context, repo *repository.Repository, mailer email.Mailer, log zerolog.Logger, pr *repository.PurchaseRequest, excludeEmail, subject, body string) {
	if mailer == nil {
		return
	}
	teamEmail, err := repo.TeamEmailForRole(ctx, model.RoleProcurement)
	if err != nil {
		log.Warn().Err(err).Msg("notify PR activity: look up procurement team email")
	}
	people := prActivityRecipients(pr, excludeEmail)
	to := strings.TrimSpace(teamEmail)
	var cc []string
	if to == "" {
		if len(people) == 0 {
			return
		}
		to, cc = people[0], people[1:]
	} else {
		cc = people
	}
	sendMailAsync(mailer, log, to, cc, subject, body)
}

// prActivityRecipients returns the PR's assignee and collaborators (deduped,
// case-insensitively, dropping blanks and excludeEmail). pr must be loaded via
// GetPurchaseRequest so Assignee/Collaborators are populated.
func prActivityRecipients(pr *repository.PurchaseRequest, excludeEmail string) []string {
	seen := map[string]bool{}
	if e := strings.ToLower(strings.TrimSpace(excludeEmail)); e != "" {
		seen[e] = true
	}
	var out []string
	add := func(addr string) {
		addr = strings.TrimSpace(addr)
		key := strings.ToLower(addr)
		if addr == "" || seen[key] {
			return
		}
		seen[key] = true
		out = append(out, addr)
	}
	if pr.Assignee != nil {
		add(pr.Assignee.Email)
	}
	for _, c := range pr.Collaborators {
		add(c.Email)
	}
	return out
}

// actorEmail / actorName describe the authenticated caller for notification text,
// tolerating a missing user in the context.
func actorEmail(r *http.Request) string {
	if u := middleware.UserFromCtx(r.Context()); u != nil {
		return u.Email
	}
	return ""
}

func actorName(r *http.Request) string {
	u := middleware.UserFromCtx(r.Context())
	if u == nil {
		return "Someone"
	}
	if strings.TrimSpace(u.Name) != "" {
		return u.Name
	}
	return u.Email
}

// prTitleOrRef is the human label for a PR in notification text.
func prTitleOrRef(pr *repository.PurchaseRequest) string {
	if pr.Title != "" {
		return pr.Title
	}
	return prDisplayRef(pr)
}

// notifyPRSubmitted tells the Procurement team a new PR was submitted. There is no
// assignee yet, so this is a team-only FYI (it becomes ready for procurement once the
// team lead approves).
func (h *PurchaseRequestsHandler) notifyPRSubmitted(r *http.Request, pr *repository.PurchaseRequest) {
	title := prTitleOrRef(pr)
	subject := fmt.Sprintf("New purchase request submitted: %s", title)
	body := fmt.Sprintf(
		"Hi,\n\n%s has submitted a new purchase request %s — \"%s\". It will be ready for "+
			"procurement once the team lead approves it.\n\nView it here:\n%s\n",
		actorName(r), prDisplayRef(pr), title, h.prURL(pr))
	notifyPRActivity(r.Context(), h.Repo, h.Mailer, h.Log, pr, actorEmail(r), subject, body)
}

// notifyTeamLeadRejected tells the Procurement team (and any assignee/collaborators)
// that the team lead rejected the PR, so it will not proceed.
func (h *PurchaseRequestsHandler) notifyTeamLeadRejected(r *http.Request, pr *repository.PurchaseRequest, notes string) {
	title := prTitleOrRef(pr)
	subject := fmt.Sprintf("Team lead rejected: %s", title)
	body := fmt.Sprintf(
		"Hi,\n\n%s has rejected purchase request %s — \"%s\" — as team lead, so it will not "+
			"proceed to procurement.\n",
		actorName(r), prDisplayRef(pr), title)
	if notes != "" {
		body += fmt.Sprintf("\nNotes:\n%s\n", notes)
	}
	body += fmt.Sprintf("\nView it here:\n%s\n", h.prURL(pr))
	notifyPRActivity(r.Context(), h.Repo, h.Mailer, h.Log, pr, actorEmail(r), subject, body)
}

// notifyRecApproval tells the Procurement team + assignee/collaborators that an
// approval card (budget/legal/security) was approved.
func (h *PurchaseRequestsHandler) notifyRecApproval(r *http.Request, pr *repository.PurchaseRequest, approvalType string) {
	title := prTitleOrRef(pr)
	label := strings.ToUpper(approvalType[:1]) + approvalType[1:]
	subject := fmt.Sprintf("%s approval granted: %s", label, title)
	body := fmt.Sprintf(
		"Hi,\n\n%s has approved the %s card on purchase request %s — \"%s\".\n\nView it here:\n%s\n",
		actorName(r), approvalType, prDisplayRef(pr), title, h.prURL(pr))
	notifyPRActivity(r.Context(), h.Repo, h.Mailer, h.Log, pr, actorEmail(r), subject, body)
}

// notifyRecComment tells the Procurement team + assignee/collaborators that a comment
// was added to an approval card.
func (h *PurchaseRequestsHandler) notifyRecComment(r *http.Request, pr *repository.PurchaseRequest, approvalType, comment string) {
	title := prTitleOrRef(pr)
	subject := fmt.Sprintf("New %s comment: %s", approvalType, title)
	body := fmt.Sprintf(
		"Hi,\n\n%s commented on the %s card of purchase request %s — \"%s\".\n",
		actorName(r), approvalType, prDisplayRef(pr), title)
	if comment != "" {
		body += fmt.Sprintf("\nComment:\n%s\n", comment)
	}
	body += fmt.Sprintf("\nView it here:\n%s\n", h.prURL(pr))
	notifyPRActivity(r.Context(), h.Repo, h.Mailer, h.Log, pr, actorEmail(r), subject, body)
}

// notifyRecommendationCreated tells the Procurement team + assignee/collaborators that
// a procurement recommendation was added.
func (h *PurchaseRequestsHandler) notifyRecommendationCreated(r *http.Request, pr *repository.PurchaseRequest) {
	title := prTitleOrRef(pr)
	subject := fmt.Sprintf("Procurement recommendation added: %s", title)
	body := fmt.Sprintf(
		"Hi,\n\n%s added a procurement recommendation to purchase request %s — \"%s\".\n\n"+
			"View it here:\n%s\n",
		actorName(r), prDisplayRef(pr), title, h.prURL(pr))
	notifyPRActivity(r.Context(), h.Repo, h.Mailer, h.Log, pr, actorEmail(r), subject, body)
}

// notifyContractAdded tells the Procurement team + assignee/collaborators that a
// contract was created on the recommendation.
func (h *PurchaseRequestsHandler) notifyContractAdded(r *http.Request, pr *repository.PurchaseRequest) {
	title := prTitleOrRef(pr)
	subject := fmt.Sprintf("Contract added: %s", title)
	body := fmt.Sprintf(
		"Hi,\n\n%s added a contract to purchase request %s — \"%s\".\n\nView it here:\n%s\n",
		actorName(r), prDisplayRef(pr), title, h.prURL(pr))
	notifyPRActivity(r.Context(), h.Repo, h.Mailer, h.Log, pr, actorEmail(r), subject, body)
}

// notifyPRAssignment emails a user who has just been made the assignee or a
// collaborator on a PR that they now own / can work on it, Cc'ing the Procurement
// team mailbox. role is "assignee" or "collaborator". No-op when the target has no
// email or assigned themselves (nobody needs to be told of their own claim).
// Best-effort. targetID is looked up fresh so the address is current.
func (h *PurchaseRequestsHandler) notifyPRAssignment(r *http.Request, pr *repository.PurchaseRequest, targetID int64, role string) {
	if h.Mailer == nil {
		return
	}
	actor := middleware.UserFromCtx(r.Context())
	if actor != nil && actor.ID == targetID {
		return
	}
	target, err := h.Repo.GetUserByID(r.Context(), targetID)
	if err != nil {
		reqLog(r).Warn().Err(err).Int64("user_id", targetID).Msg("notify PR assignment: look up user")
		return
	}
	if target == nil || strings.TrimSpace(target.Email) == "" {
		return
	}
	title := prTitleOrRef(pr)
	var subject, lead string
	if role == "collaborator" {
		subject = fmt.Sprintf("Added as collaborator: %s", title)
		lead = fmt.Sprintf("%s has added you as a collaborator on", actorName(r))
	} else {
		subject = fmt.Sprintf("Assigned to you: %s", title)
		lead = fmt.Sprintf("%s has assigned you to", actorName(r))
	}
	body := fmt.Sprintf(
		"Hi,\n\n%s purchase request %s — \"%s\". You can now work on it as part of the "+
			"Procurement team.\n\nView it here:\n%s\n",
		lead, prDisplayRef(pr), title, h.prURL(pr))
	var cc []string
	if teamEmail, err := h.Repo.TeamEmailForRole(r.Context(), model.RoleProcurement); err == nil && strings.TrimSpace(teamEmail) != "" {
		cc = []string{teamEmail}
	}
	h.sendAsyncCC(target.Email, cc, subject, body)
}

// notifyApprovalRequested emails each pending approver that their sign-off is
// requested on the PR.
func (h *PurchaseRequestsHandler) notifyApprovalRequested(pr *repository.PurchaseRequest, approvals []repository.PRApproval, requester *repository.User) {
	title := pr.Title
	if title == "" {
		title = prDisplayRef(pr)
	}
	who := requester.Name
	if who == "" {
		who = requester.Email
	}
	subject := fmt.Sprintf("Approval requested: %s", title)
	for _, a := range approvals {
		if a.Approver == nil {
			continue
		}
		body := fmt.Sprintf(
			"Hi,\n\n%s has asked you to approve purchase request %s — \"%s\".\n\n"+
				"Please review and approve or reject it here:\n%s\n",
			who, prDisplayRef(pr), title, h.prURL(pr))
		h.sendAsync(a.Approver.Email, subject, body)
	}
}

// notifyTeamLeadRequested emails the named team lead that a PR is awaiting their
// approval before procurement can act on it.
func (h *PurchaseRequestsHandler) notifyTeamLeadRequested(pr *repository.PurchaseRequest, requester *repository.User) {
	if pr.TeamLeadEmail == "" {
		return
	}
	title := pr.Title
	if title == "" {
		title = prDisplayRef(pr)
	}
	who := requester.Name
	if who == "" {
		who = requester.Email
	}
	subject := fmt.Sprintf("Team lead approval requested: %s", title)
	body := fmt.Sprintf(
		"Hi,\n\n%s has submitted purchase request %s — \"%s\" — and named you as their "+
			"team lead. It cannot proceed to procurement until you approve it.\n\n"+
			"Please review and approve or reject it here:\n%s\n",
		who, prDisplayRef(pr), title, h.prURL(pr))
	h.sendAsync(pr.TeamLeadEmail, subject, body)
}

// notifyTeamLeadReminder re-sends the pending-approval nudge to the team lead.
func (h *PurchaseRequestsHandler) notifyTeamLeadReminder(pr *repository.PurchaseRequest) {
	if pr.TeamLeadEmail == "" {
		return
	}
	title := pr.Title
	if title == "" {
		title = prDisplayRef(pr)
	}
	subject := fmt.Sprintf("Reminder — team lead approval needed: %s", title)
	body := fmt.Sprintf(
		"Hi,\n\nThis is a reminder that purchase request %s — \"%s\" — is still awaiting your "+
			"approval as team lead. It cannot proceed to procurement until you approve it.\n\n"+
			"Please review and approve or reject it here:\n%s\n",
		prDisplayRef(pr), title, h.prURL(pr))
	h.sendAsync(pr.TeamLeadEmail, subject, body)
}

// notifyProcurementTeamOfApproval emails the Procurement team's shared address that a PR
// has cleared team-lead approval and is now ready for procurement to act on. No-op
// when the Procurement team has no email configured. Best-effort.
func (h *PurchaseRequestsHandler) notifyProcurementTeamOfApproval(r *http.Request, pr *repository.PurchaseRequest, approver *repository.User) {
	teamEmail, err := h.Repo.TeamEmailForRole(r.Context(), model.RoleProcurement)
	if err != nil {
		reqLog(r).Warn().Err(err).Msg("notify procurement team: look up team email")
		return
	}
	if strings.TrimSpace(teamEmail) == "" {
		return
	}
	title := pr.Title
	if title == "" {
		title = prDisplayRef(pr)
	}
	who := approver.Name
	if who == "" {
		who = approver.Email
	}
	subject := fmt.Sprintf("Ready for procurement — team lead approved: %s", title)
	body := fmt.Sprintf(
		"Hi,\n\n%s has approved purchase request %s — \"%s\" — as team lead. "+
			"It is now ready for the Procurement team to act on.\n\n"+
			"View it here:\n%s\n",
		who, prDisplayRef(pr), title, h.prURL(pr))
	h.sendAsync(teamEmail, subject, body)
}

// notifyDecision emails the requester that an approver has made a decision.
func (h *PurchaseRequestsHandler) notifyDecision(pr *repository.PurchaseRequest, approver *repository.User, status, comment string) {
	if pr.Requester == nil {
		return
	}
	title := pr.Title
	if title == "" {
		title = prDisplayRef(pr)
	}
	who := approver.Name
	if who == "" {
		who = approver.Email
	}
	verb := "approved"
	if status == model.PRApprovalRejected {
		verb = "rejected"
	}
	subject := fmt.Sprintf("Approval %s: %s", verb, title)
	body := fmt.Sprintf("Hi,\n\n%s has %s your purchase request %s — \"%s\".\n",
		who, verb, prDisplayRef(pr), title)
	if comment != "" {
		body += fmt.Sprintf("\nComment:\n%s\n", comment)
	}
	body += fmt.Sprintf("\nView it here:\n%s\n", h.prURL(pr))
	h.sendAsync(pr.Requester.Email, subject, body)
}

// sendAsyncCC is sendAsync with additional Cc recipients (e.g. a team email).
func (h *PurchaseRequestsHandler) sendAsyncCC(to string, cc []string, subject, body string) {
	sendMailAsync(h.Mailer, h.Log, to, cc, subject, body)
}

// notifyAssignee emails the current assignee of a legal/security card that its
// review is requested, CC'ing the team email. It re-reads the recommendation so
// the address is fresh even right after the assignee was changed.
func (h *PurchaseRequestsHandler) notifyAssignee(r *http.Request, pr *repository.PurchaseRequest, approvalType, role string) {
	rec, err := h.Repo.GetRecommendation(r.Context(), pr.ID)
	if err != nil {
		reqLog(r).Warn().Err(err).Int64("pr_id", pr.ID).Msg("notify assignee: reload recommendation")
		return
	}
	if rec == nil {
		return
	}
	var assignee *repository.UserSummary
	for i := range rec.Approvals {
		if rec.Approvals[i].ApprovalType == approvalType {
			assignee = rec.Approvals[i].Assignee
			break
		}
	}
	if assignee == nil || assignee.Email == "" {
		return
	}
	teamEmail, _ := h.Repo.TeamEmailForRole(r.Context(), role)
	title := pr.Title
	if title == "" {
		title = prDisplayRef(pr)
	}
	actor := middleware.UserFromCtx(r.Context())
	who := ""
	if actor != nil {
		who = actor.Name
		if who == "" {
			who = actor.Email
		}
	}
	label := strings.ToUpper(approvalType[:1]) + approvalType[1:]
	subject := fmt.Sprintf("%s review requested: %s", label, title)
	body := fmt.Sprintf(
		"Hi,\n\n%s has assigned the %s approval of purchase request %s — \"%s\" — to you.\n\n"+
			"Please review and approve it here:\n%s\n",
		who, approvalType, prDisplayRef(pr), title, h.prURL(pr))
	var cc []string
	if strings.TrimSpace(teamEmail) != "" {
		cc = []string{teamEmail}
	}
	h.sendAsyncCC(assignee.Email, cc, subject, body)
}

// notifyBudgetApprovers emails every qualified budget approver for the PR's
// recommendation (the approvers of the resolved bracket) that their sign-off is
// requested. Best-effort — mailer errors are swallowed by sendAsync.
func (h *PurchaseRequestsHandler) notifyBudgetApprovers(r *http.Request, pr *repository.PurchaseRequest) {
	approvers, err := h.Repo.BudgetApproversForPR(r.Context(), pr.ID)
	if err != nil {
		reqLog(r).Warn().Err(err).Int64("pr_id", pr.ID).Msg("notify budget approvers: resolve")
		return
	}
	title := pr.Title
	if title == "" {
		title = prDisplayRef(pr)
	}
	subject := fmt.Sprintf("Budget approval requested: %s", title)
	body := fmt.Sprintf(
		"Hi,\n\nYour budget approval is requested for purchase request %s — \"%s\".\n\n"+
			"Please review and approve it here:\n%s\n",
		prDisplayRef(pr), title, h.prURL(pr))
	for _, a := range approvers {
		if a != nil && strings.TrimSpace(a.Email) != "" {
			h.sendAsync(a.Email, subject, body)
		}
	}
}

func (h *PurchaseRequestsHandler) UploadDocument(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.load(w, r)
	if !ok {
		return
	}
	if !h.callerCanEdit(r, pr) {
		writeError(w, http.StatusForbidden, "this request can no longer be edited")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes+1024)
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		writeError(w, http.StatusBadRequest, "invalid or too-large upload")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing file field")
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !allowedExtensions[ext] {
		writeError(w, http.StatusBadRequest, "only .pdf and .docx files are allowed")
		return
	}
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	storedPath, size, err := h.Storage.Save(pr.ID, header.Filename, file)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("save document")
		writeError(w, http.StatusInternalServerError, "failed to store file")
		return
	}
	user := middleware.UserFromCtx(r.Context())
	doc, err := h.Repo.AddDocument(r.Context(), pr.ID, header.Filename, storedPath, contentType, size, user.ID)
	if err != nil {
		_ = h.Storage.Delete(storedPath)
		reqLog(r).Error().Err(err).Msg("record document")
		writeError(w, http.StatusInternalServerError, "failed to record file")
		return
	}
	writeJSON(w, http.StatusCreated, doc)
}

func (h *PurchaseRequestsHandler) DownloadDocument(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.load(w, r)
	if !ok {
		return
	}
	if !h.callerCanView(r, pr) {
		writeError(w, http.StatusForbidden, "not allowed to view this request")
		return
	}
	docID, err := parseID(r, "docID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid document id")
		return
	}
	doc, err := h.Repo.GetDocument(r.Context(), pr.ID, docID)
	if err != nil {
		writeError(w, http.StatusNotFound, "document not found")
		return
	}
	f, err := h.Storage.Open(doc.StoredPath)
	if err != nil {
		reqLog(r).Error().Err(err).Str("path", doc.StoredPath).Msg("open document")
		writeError(w, http.StatusInternalServerError, "failed to open file")
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", doc.ContentType)
	w.Header().Set("Content-Disposition", "attachment; filename=\""+sanitizeHeaderFilename(doc.Filename)+"\"")
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, f); err != nil {
		reqLog(r).Warn().Err(err).Msg("stream document")
	}
}

func (h *PurchaseRequestsHandler) DeleteDocument(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.load(w, r)
	if !ok {
		return
	}
	if !h.callerCanEdit(r, pr) {
		writeError(w, http.StatusForbidden, "this request can no longer be edited")
		return
	}
	docID, err := parseID(r, "docID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid document id")
		return
	}
	doc, err := h.Repo.GetDocument(r.Context(), pr.ID, docID)
	if err != nil {
		writeError(w, http.StatusNotFound, "document not found")
		return
	}
	if err := h.Repo.DeleteDocument(r.Context(), pr.ID, docID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete document")
		return
	}
	_ = h.Storage.Delete(doc.StoredPath)
	w.WriteHeader(http.StatusNoContent)
}

// load fetches the request by URL id, writing the appropriate error response if
// not found. The boolean is false when the caller should stop.
func (h *PurchaseRequestsHandler) load(w http.ResponseWriter, r *http.Request) (*repository.PurchaseRequest, bool) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request id")
		return nil, false
	}
	pr, err := h.Repo.GetPurchaseRequest(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "request not found")
			return nil, false
		}
		reqLog(r).Error().Err(err).Msg("get purchase request")
		writeError(w, http.StatusInternalServerError, "failed to load request")
		return nil, false
	}
	return pr, true
}

// sanitizeHeaderFilename strips characters that would break the Content-Disposition header.
func sanitizeHeaderFilename(name string) string {
	name = strings.ReplaceAll(name, "\"", "")
	name = strings.ReplaceAll(name, "\\", "")
	name = strings.ReplaceAll(name, "\r", "")
	name = strings.ReplaceAll(name, "\n", "")
	return name
}
