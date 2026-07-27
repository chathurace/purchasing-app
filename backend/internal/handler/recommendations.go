package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/cs/purchasing-app/internal/middleware"
	"github.com/cs/purchasing-app/internal/model"
	"github.com/cs/purchasing-app/internal/repository"
	"github.com/jackc/pgx/v5"
)

// Procurement recommendation endpoints. These hang off a purchase request and
// reuse the PurchaseRequestsHandler so they share its repo, storage and the
// PR view/permission helpers.

type recommendationInput struct {
	VendorID       int64    `json:"vendor_id"`
	Description    string   `json:"description"`
	EstimatedValue float64  `json:"estimated_value"`
	Currency       string   `json:"currency"`
	EngagementType string   `json:"engagement_type"`
	RequiredTypes  []string `json:"required_types"`
}

// toRepoInput validates and builds the repository input from the decoded request.
// It requires a vendor and ≥1 approval type; commercial fields are optional. It
// returns the input, a user-facing error message (empty on success), and whether
// it succeeded.
func (in recommendationInput) toRepoInput() (repository.RecommendationInput, string) {
	types, ok := normalizeRequiredTypes(in.RequiredTypes)
	if !ok {
		return repository.RecommendationInput{}, "select at least one approval (budget, legal or security)"
	}
	if in.VendorID == 0 {
		return repository.RecommendationInput{}, "a vendor is required"
	}
	if in.EstimatedValue < 0 {
		return repository.RecommendationInput{}, "estimated value cannot be negative"
	}
	return repository.RecommendationInput{
		VendorID:       in.VendorID,
		Description:    strings.TrimSpace(in.Description),
		EstimatedValue: in.EstimatedValue,
		Currency:       strings.TrimSpace(in.Currency),
		EngagementType: strings.TrimSpace(in.EngagementType),
		RequiredTypes:  types,
	}, ""
}

// normalizeRequiredTypes validates and dedupes the requested approval types,
// returning them in canonical order (budget, legal, security). Requires ≥1.
func normalizeRequiredTypes(in []string) ([]string, bool) {
	set := map[string]bool{}
	for _, t := range in {
		if !model.IsRecApprovalType(t) {
			return nil, false
		}
		set[t] = true
	}
	out := []string{}
	for _, t := range model.RecApprovalTypes {
		if set[t] {
			out = append(out, t)
		}
	}
	return out, len(out) > 0
}

// reqHasBudget reports whether the budget card is among the required approvals.
func reqHasBudget(types []string) bool {
	for _, t := range types {
		if t == model.RecApprovalBudget {
			return true
		}
	}
	return false
}

// loadViewablePR loads the PR by {id} and enforces view access.
func (h *PurchaseRequestsHandler) loadViewablePR(w http.ResponseWriter, r *http.Request) (*repository.PurchaseRequest, bool) {
	pr, ok := h.load(w, r)
	if !ok {
		return nil, false
	}
	if !h.callerCanView(r, pr) {
		writeError(w, http.StatusForbidden, "not allowed to view this request")
		return nil, false
	}
	return pr, true
}

// vendorHasQuotation reports whether the vendor has a quotation on the PR (the
// recommendation may only name a vendor that has quoted).
func (h *PurchaseRequestsHandler) vendorHasQuotation(r *http.Request, prID, vendorID int64) (bool, error) {
	quotes, err := h.Repo.ListQuotations(r.Context(), &prID)
	if err != nil {
		reqLog(r).Error().Err(err).Int64("pr_id", prID).Msg("validate vendor: list quotations")
		return false, err
	}
	for _, q := range quotes {
		if q.VendorID == vendorID {
			return true, nil
		}
	}
	return false, nil
}

// CreateRecommendation adds the (single) procurement recommendation to a PR.
func (h *PurchaseRequestsHandler) CreateRecommendation(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasProcurementAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "procurement access required")
		return
	}
	pr, ok := h.load(w, r)
	if !ok {
		return
	}
	if !model.IsTeamLeadApproved(pr.TeamLeadStatus) {
		writeError(w, http.StatusConflict, "purchase request is awaiting team lead approval")
		return
	}
	if code, msg, ok := assignmentWorkGate(r, pr); !ok {
		writeError(w, code, msg)
		return
	}
	if pr.Recommendation != nil {
		writeError(w, http.StatusConflict, "this request already has a recommendation")
		return
	}
	var in recommendationInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	repoIn, errMsg := in.toRepoInput()
	if errMsg != "" {
		writeError(w, http.StatusBadRequest, errMsg)
		return
	}
	hasQuote, err := h.vendorHasQuotation(r, pr.ID, repoIn.VendorID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to validate vendor")
		return
	}
	if !hasQuote {
		writeError(w, http.StatusBadRequest, "the vendor must have a quotation on this request")
		return
	}
	user := middleware.UserFromCtx(r.Context())
	if _, err := h.Repo.CreateRecommendation(r.Context(), pr.ID, repoIn, user.ID); err != nil {
		reqLog(r).Error().Err(err).Msg("create recommendation")
		writeError(w, http.StatusInternalServerError, "failed to create recommendation")
		return
	}
	recordProcessEvent(r, h.Repo, pr.ID, model.ProcessCreateRecommendation, "")
	ensureCollaborator(r, h.Repo, pr.ID)
	if reqHasBudget(repoIn.RequiredTypes) {
		h.notifyBudgetApprovers(r, pr)
	}
	h.notifyRecommendationCreated(r, pr)
	h.reloadPR(w, r, pr.ID)
}

// UpdateRecommendation replaces the recommendation's vendor / description /
// required approvals, resetting every card to pending.
func (h *PurchaseRequestsHandler) UpdateRecommendation(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasProcurementAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "procurement access required")
		return
	}
	pr, ok := h.load(w, r)
	if !ok {
		return
	}
	if pr.Recommendation == nil {
		writeError(w, http.StatusNotFound, "no recommendation on this request")
		return
	}
	var in recommendationInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	repoIn, errMsg := in.toRepoInput()
	if errMsg != "" {
		writeError(w, http.StatusBadRequest, errMsg)
		return
	}
	hasQuote, err := h.vendorHasQuotation(r, pr.ID, repoIn.VendorID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to validate vendor")
		return
	}
	if !hasQuote {
		writeError(w, http.StatusBadRequest, "the vendor must have a quotation on this request")
		return
	}
	paths, err := h.Repo.UpdateRecommendation(r.Context(), pr.ID, repoIn)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("update recommendation")
		writeError(w, http.StatusInternalServerError, "failed to update recommendation")
		return
	}
	for _, p := range paths {
		_ = h.Storage.Delete(p)
	}
	recordProcessEvent(r, h.Repo, pr.ID, model.ProcessUpdateRecommendation, "")
	ensureCollaborator(r, h.Repo, pr.ID)
	// The estimated value may have changed the resolved budget approver(s), and
	// editing resets every card to pending, so re-notify them.
	if reqHasBudget(repoIn.RequiredTypes) {
		h.notifyBudgetApprovers(r, pr)
	}
	h.reloadPR(w, r, pr.ID)
}

// DeleteRecommendation removes a PR's recommendation.
func (h *PurchaseRequestsHandler) DeleteRecommendation(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasProcurementAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "procurement access required")
		return
	}
	pr, ok := h.load(w, r)
	if !ok {
		return
	}
	if pr.Recommendation == nil {
		writeError(w, http.StatusNotFound, "no recommendation on this request")
		return
	}
	paths, err := h.Repo.DeleteRecommendation(r.Context(), pr.ID)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("delete recommendation")
		writeError(w, http.StatusInternalServerError, "failed to delete recommendation")
		return
	}
	for _, p := range paths {
		_ = h.Storage.Delete(p)
	}
	recordProcessEvent(r, h.Repo, pr.ID, model.ProcessDeleteRecommendation, "")
	ensureCollaborator(r, h.Repo, pr.ID)
	w.WriteHeader(http.StatusNoContent)
}

// RequestRecApproval adds (requires) a single approval card to the recommendation
// — the "Request X approval" buttons. Procurement only.
func (h *PurchaseRequestsHandler) RequestRecApproval(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasProcurementAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "procurement access required")
		return
	}
	pr, ok := h.load(w, r)
	if !ok {
		return
	}
	if pr.Recommendation == nil {
		writeError(w, http.StatusNotFound, "no recommendation on this request")
		return
	}
	approvalType := urlParam(r, "type")
	if !model.IsRecApprovalType(approvalType) {
		writeError(w, http.StatusBadRequest, "approval type must be budget, legal or security")
		return
	}
	if err := h.Repo.AddRecApprovalCard(r.Context(), pr.ID, approvalType); err != nil {
		if errors.Is(err, repository.ErrInvalidState) {
			writeError(w, http.StatusConflict, "the "+approvalType+" approval is already requested")
			return
		}
		reqLog(r).Error().Err(err).Msg("request recommendation approval")
		writeError(w, http.StatusInternalServerError, "failed to request approval")
		return
	}
	recordProcessEvent(r, h.Repo, pr.ID, model.ProcessRequestRecApproval, approvalType)
	ensureCollaborator(r, h.Repo, pr.ID)
	if approvalType == model.RecApprovalBudget {
		h.notifyBudgetApprovers(r, pr)
	}
	h.reloadPR(w, r, pr.ID)
}

// RemoveRecApproval removes a single approval card from the recommendation — the
// "Remove X approval" button (confirmed on the client). Procurement only; a
// recommendation must keep at least one card.
func (h *PurchaseRequestsHandler) RemoveRecApproval(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasProcurementAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "procurement access required")
		return
	}
	pr, ok := h.load(w, r)
	if !ok {
		return
	}
	if pr.Recommendation == nil {
		writeError(w, http.StatusNotFound, "no recommendation on this request")
		return
	}
	approvalType := urlParam(r, "type")
	if !model.IsRecApprovalType(approvalType) {
		writeError(w, http.StatusBadRequest, "approval type must be budget, legal or security")
		return
	}
	paths, err := h.Repo.RemoveRecApprovalCard(r.Context(), pr.ID, approvalType)
	if err != nil {
		if errors.Is(err, repository.ErrInvalidState) {
			writeError(w, http.StatusConflict, "a recommendation must keep at least one approval")
			return
		}
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "the "+approvalType+" approval is not required on this recommendation")
			return
		}
		reqLog(r).Error().Err(err).Msg("remove recommendation approval")
		writeError(w, http.StatusInternalServerError, "failed to remove approval")
		return
	}
	for _, p := range paths {
		_ = h.Storage.Delete(p)
	}
	recordProcessEvent(r, h.Repo, pr.ID, model.ProcessRemoveRecApproval, approvalType)
	ensureCollaborator(r, h.Repo, pr.ID)
	h.reloadPR(w, r, pr.ID)
}

// CreateRecommendationContract drafts the contract attached to a PR's
// recommendation (the optional contract card). The given description becomes the
// contract terms; the PDF is uploaded separately against the returned contract
// (POST /contracts/{id}/documents). Procurement only; one contract per recommendation.
func (h *PurchaseRequestsHandler) CreateRecommendationContract(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasProcurementAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "procurement access required")
		return
	}
	pr, ok := h.load(w, r)
	if !ok {
		return
	}
	if pr.Recommendation == nil {
		writeError(w, http.StatusNotFound, "no recommendation on this request")
		return
	}
	if pr.Recommendation.ContractID != nil {
		writeError(w, http.StatusConflict, "this recommendation already has a contract")
		return
	}
	var in struct {
		Description string `json:"description"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	user := middleware.UserFromCtx(r.Context())
	c, err := h.Repo.CreateRecommendationContract(r.Context(), pr.ID, strings.TrimSpace(in.Description), user.ID)
	if err != nil {
		if errors.Is(err, repository.ErrInvalidState) {
			writeError(w, http.StatusConflict, "this recommendation already has a contract")
			return
		}
		reqLog(r).Error().Err(err).Msg("create recommendation contract")
		writeError(w, http.StatusInternalServerError, "failed to create contract")
		return
	}
	recordProcessEvent(r, h.Repo, pr.ID, model.ProcessAddDraftContract, model.QualifierFromRecommendation)
	ensureCollaborator(r, h.Repo, pr.ID)
	h.notifyContractAdded(r, pr)
	writeJSON(w, http.StatusCreated, c)
}

// DeleteRecommendationContract removes the contract attached to a PR's
// recommendation (only while it is still a draft) and unlinks it. Procurement only.
func (h *PurchaseRequestsHandler) DeleteRecommendationContract(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasProcurementAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "procurement access required")
		return
	}
	pr, ok := h.load(w, r)
	if !ok {
		return
	}
	if pr.Recommendation == nil || pr.Recommendation.ContractID == nil {
		writeError(w, http.StatusNotFound, "no contract on this recommendation")
		return
	}
	paths, err := h.Repo.DeleteRecommendationContract(r.Context(), pr.ID)
	if err != nil {
		if errors.Is(err, repository.ErrInvalidState) {
			writeError(w, http.StatusConflict, "only a draft contract can be removed here; manage it from the contract page")
			return
		}
		reqLog(r).Error().Err(err).Msg("delete recommendation contract")
		writeError(w, http.StatusInternalServerError, "failed to remove contract")
		return
	}
	for _, p := range paths {
		_ = h.Storage.Delete(p)
	}
	recordProcessEvent(r, h.Repo, pr.ID, model.ProcessDeleteContract, "")
	ensureCollaborator(r, h.Repo, pr.ID)
	h.reloadPR(w, r, pr.ID)
}

// SetRecommendationRFI sets the RFI description on a PR's recommendation (the
// optional RFI card). Procurement only. The PDF attachments are managed separately.
func (h *PurchaseRequestsHandler) SetRecommendationRFI(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasProcurementAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "procurement access required")
		return
	}
	pr, ok := h.load(w, r)
	if !ok {
		return
	}
	if pr.Recommendation == nil {
		writeError(w, http.StatusNotFound, "no recommendation on this request")
		return
	}
	var in struct {
		Description string `json:"description"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.Repo.SetRecommendationRFIDescription(r.Context(), pr.ID, strings.TrimSpace(in.Description)); err != nil {
		reqLog(r).Error().Err(err).Msg("set recommendation rfi")
		writeError(w, http.StatusInternalServerError, "failed to update RFI")
		return
	}
	recordProcessEvent(r, h.Repo, pr.ID, model.ProcessRaiseRFI, "")
	ensureCollaborator(r, h.Repo, pr.ID)
	h.reloadPR(w, r, pr.ID)
}

// DeleteRecommendationRFI clears a PR's RFI: blanks the description and removes
// all RFI attachments. Procurement only.
func (h *PurchaseRequestsHandler) DeleteRecommendationRFI(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasProcurementAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "procurement access required")
		return
	}
	pr, ok := h.load(w, r)
	if !ok {
		return
	}
	if pr.Recommendation == nil {
		writeError(w, http.StatusNotFound, "no recommendation on this request")
		return
	}
	paths, err := h.Repo.ClearRecommendationRFI(r.Context(), pr.ID)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("clear recommendation rfi")
		writeError(w, http.StatusInternalServerError, "failed to remove RFI")
		return
	}
	for _, p := range paths {
		_ = h.Storage.Delete(p)
	}
	recordProcessEvent(r, h.Repo, pr.ID, model.ProcessClearRFI, "")
	ensureCollaborator(r, h.Repo, pr.ID)
	h.reloadPR(w, r, pr.ID)
}

// UploadRecRFIDocument attaches a PDF to a PR's RFI. Procurement only.
func (h *PurchaseRequestsHandler) UploadRecRFIDocument(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasProcurementAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "procurement access required")
		return
	}
	pr, ok := h.load(w, r)
	if !ok {
		return
	}
	if pr.Recommendation == nil {
		writeError(w, http.StatusNotFound, "no recommendation on this request")
		return
	}
	doc, ok := saveUploadedDoc(w, r, h.Repo, h.Storage, pr.ID, model.OwnerRecommendationRFI, pr.Recommendation.ID, pdfOnly)
	if !ok {
		return
	}
	writeJSON(w, http.StatusCreated, doc)
}

// DownloadRecRFIDocument streams an RFI attachment. Any PR viewer.
func (h *PurchaseRequestsHandler) DownloadRecRFIDocument(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.loadViewablePR(w, r)
	if !ok {
		return
	}
	if pr.Recommendation == nil {
		writeError(w, http.StatusNotFound, "no recommendation on this request")
		return
	}
	docID, err := parseID(r, "docID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid document id")
		return
	}
	downloadOwnedDoc(w, r, h.Repo, h.Storage, model.OwnerRecommendationRFI, pr.Recommendation.ID, docID)
}

// DeleteRecRFIDocument removes an RFI attachment. Procurement only.
func (h *PurchaseRequestsHandler) DeleteRecRFIDocument(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasProcurementAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "procurement access required")
		return
	}
	pr, ok := h.load(w, r)
	if !ok {
		return
	}
	if pr.Recommendation == nil {
		writeError(w, http.StatusNotFound, "no recommendation on this request")
		return
	}
	docID, err := parseID(r, "docID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid document id")
		return
	}
	deleteOwnedDoc(w, r, h.Repo, h.Storage, model.OwnerRecommendationRFI, pr.Recommendation.ID, docID)
}

// SetRecApproval toggles a recommendation approval card (approve / revert),
// gated by the card's actor (legal/security role or the budget owner).
func (h *PurchaseRequestsHandler) SetRecApproval(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.loadViewablePR(w, r)
	if !ok {
		return
	}
	if pr.Recommendation == nil {
		writeError(w, http.StatusNotFound, "no recommendation on this request")
		return
	}
	approvalType := urlParam(r, "type")
	if !model.IsRecApprovalType(approvalType) {
		writeError(w, http.StatusBadRequest, "approval type must be budget, legal or security")
		return
	}
	if approvalType == model.RecApprovalBudget {
		// Budget approval is a serial chain — decisions go through the step endpoints.
		writeError(w, http.StatusBadRequest, "budget approval is recorded per step (see budget-steps)")
		return
	}
	card := findApproval(pr.Recommendation, approvalType)
	if card == nil {
		writeError(w, http.StatusBadRequest, "the "+approvalType+" approval is not required on this recommendation")
		return
	}
	can, err := h.canApproveRecType(r, pr, card)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check permission")
		return
	}
	if !can {
		writeError(w, http.StatusForbidden, "only the assigned reviewer can approve the "+approvalType+" card")
		return
	}
	var in struct {
		Approved bool `json:"approved"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	user := middleware.UserFromCtx(r.Context())
	if in.Approved {
		err = h.Repo.SetRecApproval(r.Context(), pr.ID, approvalType, user.ID)
	} else {
		err = h.Repo.ClearRecApproval(r.Context(), pr.ID, approvalType)
	}
	if err != nil {
		reqLog(r).Error().Err(err).Msg("set recommendation approval")
		writeError(w, http.StatusInternalServerError, "failed to update approval")
		return
	}
	if action, ok := model.RecApprovalAction(approvalType); ok {
		qualifier := model.QualifierRevert
		if in.Approved {
			qualifier = model.QualifierApprove
		}
		recordProcessEvent(r, h.Repo, pr.ID, action, qualifier)
	}
	if in.Approved {
		h.notifyRecApproval(r, pr, approvalType)
	}
	h.reloadPR(w, r, pr.ID)
}

// AddRecComment appends a comment to an approval card (text now, documents via a
// follow-up upload). Gated by the card's actor.
func (h *PurchaseRequestsHandler) AddRecComment(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.loadViewablePR(w, r)
	if !ok {
		return
	}
	if pr.Recommendation == nil {
		writeError(w, http.StatusNotFound, "no recommendation on this request")
		return
	}
	var in struct {
		ApprovalType string `json:"approval_type"`
		BudgetStepID *int64 `json:"budget_step_id"`
		Comment      string `json:"comment"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !model.IsRecApprovalType(in.ApprovalType) {
		writeError(w, http.StatusBadRequest, "approval type must be budget, legal or security")
		return
	}
	// Budget comments hang off a specific step; commenting is allowed for anyone
	// qualified on that step (its approver, or a budget-unit approver for the base)
	// or a procurement user managing the chain.
	if in.BudgetStepID != nil {
		if in.ApprovalType != model.RecApprovalBudget {
			writeError(w, http.StatusBadRequest, "budget_step_id is only valid for the budget card")
			return
		}
		step := findBudgetStep(pr.Recommendation, *in.BudgetStepID)
		if step == nil {
			writeError(w, http.StatusBadRequest, "unknown budget step")
			return
		}
		if !h.canCommentOnBudgetStep(r, pr, step) {
			writeError(w, http.StatusForbidden, "you cannot comment on this budget step")
			return
		}
	} else {
		can, err := h.canActOnRecType(r, pr, in.ApprovalType)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to check permission")
			return
		}
		if !can {
			writeError(w, http.StatusForbidden, "you cannot comment on the "+in.ApprovalType+" approval")
			return
		}
	}
	user := middleware.UserFromCtx(r.Context())
	comment, err := h.Repo.AddRecComment(r.Context(), pr.ID, in.ApprovalType, in.BudgetStepID, user.ID, strings.TrimSpace(in.Comment))
	if err != nil {
		reqLog(r).Error().Err(err).Msg("add recommendation comment")
		writeError(w, http.StatusInternalServerError, "failed to add comment")
		return
	}
	h.notifyRecComment(r, pr, in.ApprovalType, strings.TrimSpace(in.Comment))
	writeJSON(w, http.StatusCreated, comment)
}

// findApproval returns the approval card of the given type on a recommendation,
// or nil when that card is not required.
func findApproval(rec *repository.Recommendation, approvalType string) *repository.RecApproval {
	if rec == nil {
		return nil
	}
	for i := range rec.Approvals {
		if rec.Approvals[i].ApprovalType == approvalType {
			return &rec.Approvals[i]
		}
	}
	return nil
}

// SetRecAssignee sets (or clears) the assignee of a legal/security approval card.
// Any member of the card's team, or any procurement user, may assign it; the assignee
// must be a member of that team. When notify is true and an assignee is set, the
// assignee is emailed (CC the team email). The budget card has no assignee.
func (h *PurchaseRequestsHandler) SetRecAssignee(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.loadViewablePR(w, r)
	if !ok {
		return
	}
	if pr.Recommendation == nil {
		writeError(w, http.StatusNotFound, "no recommendation on this request")
		return
	}
	approvalType := urlParam(r, "type")
	role, ok := roleForAssignableType(approvalType)
	if !ok {
		writeError(w, http.StatusBadRequest, "only the legal and security cards have an assignee")
		return
	}
	if !h.canAssignRecType(r.Context(), approvalType) {
		writeError(w, http.StatusForbidden, "you cannot assign the "+approvalType+" card")
		return
	}
	if findApproval(pr.Recommendation, approvalType) == nil {
		writeError(w, http.StatusBadRequest, "the "+approvalType+" approval is not required on this recommendation")
		return
	}
	var in struct {
		AssigneeID *int64 `json:"assignee_id"`
		Notify     bool   `json:"notify"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	// A named assignee must be an active member of the card's team.
	if in.AssigneeID != nil {
		member, err := h.Repo.UserHasRole(r.Context(), *in.AssigneeID, role)
		if err != nil {
			reqLog(r).Error().Err(err).Msg("check assignee membership")
			writeError(w, http.StatusInternalServerError, "failed to set assignee")
			return
		}
		if !member {
			writeError(w, http.StatusBadRequest, "the assignee must be a member of the "+approvalType+" team")
			return
		}
	}
	if err := h.Repo.SetRecAssignee(r.Context(), pr.ID, approvalType, in.AssigneeID); err != nil {
		reqLog(r).Error().Err(err).Msg("set recommendation assignee")
		writeError(w, http.StatusInternalServerError, "failed to set assignee")
		return
	}
	qualifier := model.QualifierUnassign
	if in.AssigneeID != nil {
		qualifier = model.QualifierAssign
	}
	if action, ok := model.RecAssignAction(approvalType); ok {
		recordProcessEvent(r, h.Repo, pr.ID, action, qualifier)
	}
	if in.Notify && in.AssigneeID != nil {
		h.notifyAssignee(r, pr, approvalType, role)
	}
	h.reloadPR(w, r, pr.ID)
}

// RemindRecAssignee re-sends the assignment notification to a card's current
// assignee (CC the team email). Same actor rule as setting the assignee.
func (h *PurchaseRequestsHandler) RemindRecAssignee(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.loadViewablePR(w, r)
	if !ok {
		return
	}
	if pr.Recommendation == nil {
		writeError(w, http.StatusNotFound, "no recommendation on this request")
		return
	}
	approvalType := urlParam(r, "type")
	role, ok := roleForAssignableType(approvalType)
	if !ok {
		writeError(w, http.StatusBadRequest, "only the legal and security cards have an assignee")
		return
	}
	if !h.canAssignRecType(r.Context(), approvalType) {
		writeError(w, http.StatusForbidden, "you cannot remind the "+approvalType+" assignee")
		return
	}
	card := findApproval(pr.Recommendation, approvalType)
	if card == nil || card.Assignee == nil {
		writeError(w, http.StatusBadRequest, "this card has no assignee to remind")
		return
	}
	h.notifyAssignee(r, pr, approvalType, role)
	w.WriteHeader(http.StatusNoContent)
}

// RemindBudgetApprovers re-sends the budget approval notification to every
// qualified budget approver of the PR's recommendation. Procurement may trigger it
// (mirroring the assignee reminder for legal/security). The budget card must be
// required.
func (h *PurchaseRequestsHandler) RemindBudgetApprovers(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasProcurementAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "procurement access required")
		return
	}
	pr, ok := h.loadViewablePR(w, r)
	if !ok {
		return
	}
	if pr.Recommendation == nil {
		writeError(w, http.StatusNotFound, "no recommendation on this request")
		return
	}
	if findApproval(pr.Recommendation, model.RecApprovalBudget) == nil {
		writeError(w, http.StatusBadRequest, "the budget approval is not required on this recommendation")
		return
	}
	h.notifyBudgetApprovers(r, pr)
	w.WriteHeader(http.StatusNoContent)
}

// =====================================================================
// Budget approval chain (serial steps on the budget card)
// =====================================================================

// findBudgetStep returns the budget step with the given id on a recommendation's
// budget card, or nil.
func findBudgetStep(rec *repository.Recommendation, stepID int64) *repository.BudgetStep {
	card := findApproval(rec, model.RecApprovalBudget)
	if card == nil {
		return nil
	}
	for i := range card.BudgetSteps {
		if card.BudgetSteps[i].ID == stepID {
			return &card.BudgetSteps[i]
		}
	}
	return nil
}

// canBudgetStepActor reports whether the caller is the approver of a budget step
// (ignoring the serial order, which the repository enforces): admins pass; the base
// step is governed by budget-unit membership; an additional step matches on its
// named approver email (case-insensitive).
func (h *PurchaseRequestsHandler) canBudgetStepActor(r *http.Request, pr *repository.PurchaseRequest, step *repository.BudgetStep) (bool, error) {
	ctx := r.Context()
	if middleware.HasRole(ctx, model.RoleAdmin) {
		return true, nil
	}
	user := middleware.UserFromCtx(ctx)
	if user == nil {
		return false, nil
	}
	if step.IsBase {
		return h.Repo.IsBudgetApproverForPR(ctx, pr.ID, user.ID)
	}
	return step.ApproverEmail != "" &&
		strings.EqualFold(strings.TrimSpace(user.Email), strings.TrimSpace(step.ApproverEmail)), nil
}

// canCommentOnBudgetStep reports whether the caller may comment on a budget step:
// the step's approver, or any procurement user managing the chain.
func (h *PurchaseRequestsHandler) canCommentOnBudgetStep(r *http.Request, pr *repository.PurchaseRequest, step *repository.BudgetStep) bool {
	if middleware.HasProcurementAccess(r.Context()) {
		return true
	}
	ok, err := h.canBudgetStepActor(r, pr, step)
	if err != nil {
		reqLog(r).Error().Err(err).Int64("pr_id", pr.ID).Msg("check budget step actor")
	}
	return ok
}

// loadBudgetChainPR loads the PR (must have a recommendation with a budget card)
// and enforces the procurement work gate — the shared preamble for the chain
// management endpoints (add / edit / remove / remind).
func (h *PurchaseRequestsHandler) loadBudgetChainPR(w http.ResponseWriter, r *http.Request) (*repository.PurchaseRequest, bool) {
	if !middleware.HasProcurementAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "procurement access required")
		return nil, false
	}
	pr, ok := h.load(w, r)
	if !ok {
		return nil, false
	}
	if !model.IsTeamLeadApproved(pr.TeamLeadStatus) {
		writeError(w, http.StatusConflict, "purchase request is awaiting team lead approval")
		return nil, false
	}
	if code, msg, ok := assignmentWorkGate(r, pr); !ok {
		writeError(w, code, msg)
		return nil, false
	}
	if pr.Recommendation == nil {
		writeError(w, http.StatusNotFound, "no recommendation on this request")
		return nil, false
	}
	if findApproval(pr.Recommendation, model.RecApprovalBudget) == nil {
		writeError(w, http.StatusBadRequest, "the budget approval is not required on this recommendation")
		return nil, false
	}
	return pr, true
}

// AddBudgetStep appends a named budget approval step to the chain. Procurement only.
func (h *PurchaseRequestsHandler) AddBudgetStep(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.loadBudgetChainPR(w, r)
	if !ok {
		return
	}
	var in struct {
		ApproverName  string `json:"approver_name"`
		ApproverEmail string `json:"approver_email"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	email := strings.TrimSpace(in.ApproverEmail)
	if email == "" || !strings.Contains(email, "@") {
		writeError(w, http.StatusBadRequest, "a valid approver email is required")
		return
	}
	step, err := h.Repo.AddBudgetStep(r.Context(), pr.ID, strings.TrimSpace(in.ApproverName), email)
	if err != nil {
		if errors.Is(err, repository.ErrInvalidState) {
			writeError(w, http.StatusConflict, "the budget approval is not required on this recommendation")
			return
		}
		reqLog(r).Error().Err(err).Msg("add budget step")
		writeError(w, http.StatusInternalServerError, "failed to add budget approval step")
		return
	}
	recordProcessEvent(r, h.Repo, pr.ID, model.ProcessUpdateBudgetChain, model.QualifierAdd)
	ensureCollaborator(r, h.Repo, pr.ID)
	// A newly added step is only actionable once its predecessors are approved; notify
	// it now only if it is already the current step (i.e. the base was already approved).
	h.notifyBudgetStepIfCurrent(r, pr, step.Position)
	h.reloadPR(w, r, pr.ID)
}

// UpdateBudgetStep edits an additional step's named approver. Procurement only.
func (h *PurchaseRequestsHandler) UpdateBudgetStep(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.loadBudgetChainPR(w, r)
	if !ok {
		return
	}
	stepID, err := parseID(r, "stepID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid step id")
		return
	}
	var in struct {
		ApproverName  string `json:"approver_name"`
		ApproverEmail string `json:"approver_email"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	email := strings.TrimSpace(in.ApproverEmail)
	if email == "" || !strings.Contains(email, "@") {
		writeError(w, http.StatusBadRequest, "a valid approver email is required")
		return
	}
	if err := h.Repo.UpdateBudgetStep(r.Context(), pr.ID, stepID, strings.TrimSpace(in.ApproverName), email); err != nil {
		if errors.Is(err, repository.ErrInvalidState) {
			writeError(w, http.StatusConflict, "only a pending additional step can be edited")
			return
		}
		reqLog(r).Error().Err(err).Msg("update budget step")
		writeError(w, http.StatusInternalServerError, "failed to update budget approval step")
		return
	}
	recordProcessEvent(r, h.Repo, pr.ID, model.ProcessUpdateBudgetChain, model.QualifierUpdate)
	ensureCollaborator(r, h.Repo, pr.ID)
	h.reloadPR(w, r, pr.ID)
}

// DeleteBudgetStep removes an additional, still-pending step. Procurement only.
func (h *PurchaseRequestsHandler) DeleteBudgetStep(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.loadBudgetChainPR(w, r)
	if !ok {
		return
	}
	stepID, err := parseID(r, "stepID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid step id")
		return
	}
	if err := h.Repo.DeleteBudgetStep(r.Context(), pr.ID, stepID); err != nil {
		if errors.Is(err, repository.ErrInvalidState) {
			writeError(w, http.StatusConflict, "only a pending additional step can be removed")
			return
		}
		reqLog(r).Error().Err(err).Msg("delete budget step")
		writeError(w, http.StatusInternalServerError, "failed to remove budget approval step")
		return
	}
	recordProcessEvent(r, h.Repo, pr.ID, model.ProcessUpdateBudgetChain, model.QualifierRemove)
	ensureCollaborator(r, h.Repo, pr.ID)
	h.reloadPR(w, r, pr.ID)
}

// SetBudgetStepDecision records a step's approve / reject / revert decision. The
// step's approver acts (admins pass); the repository enforces the serial order.
func (h *PurchaseRequestsHandler) SetBudgetStepDecision(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.loadViewablePR(w, r)
	if !ok {
		return
	}
	if pr.Recommendation == nil {
		writeError(w, http.StatusNotFound, "no recommendation on this request")
		return
	}
	stepID, err := parseID(r, "stepID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid step id")
		return
	}
	step := findBudgetStep(pr.Recommendation, stepID)
	if step == nil {
		writeError(w, http.StatusNotFound, "unknown budget step")
		return
	}
	var in struct {
		Decision string `json:"decision"` // approve | reject | revert
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if in.Decision != "approve" && in.Decision != "reject" && in.Decision != "revert" {
		writeError(w, http.StatusBadRequest, "decision must be approve, reject or revert")
		return
	}
	can, err := h.canBudgetStepActor(r, pr, step)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check permission")
		return
	}
	if !can {
		writeError(w, http.StatusForbidden, "only this step's approver can decide it")
		return
	}
	user := middleware.UserFromCtx(r.Context())
	updated, err := h.Repo.SetBudgetStepDecision(r.Context(), pr.ID, stepID, in.Decision, user.ID)
	if err != nil {
		if errors.Is(err, repository.ErrInvalidState) {
			writeError(w, http.StatusConflict, "this step cannot be decided yet (the previous step must be approved and the next step must be undecided)")
			return
		}
		reqLog(r).Error().Err(err).Msg("set budget step decision")
		writeError(w, http.StatusInternalServerError, "failed to record decision")
		return
	}
	qualifier := model.QualifierRevert
	switch in.Decision {
	case "approve":
		qualifier = model.QualifierApprove
	case "reject":
		qualifier = model.QualifierReject
	}
	recordProcessEvent(r, h.Repo, pr.ID, model.ProcessRecApprovalBudget, qualifier)
	ensureCollaborator(r, h.Repo, pr.ID)
	// Approving this step hands off to the next one — notify its approver.
	if in.Decision == "approve" {
		h.notifyBudgetStepIfCurrent(r, pr, updated.Position+1)
	}
	h.reloadPR(w, r, pr.ID)
}

// RemindBudgetStep re-sends the approval request to an additional step's approver.
// Procurement only.
func (h *PurchaseRequestsHandler) RemindBudgetStep(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.loadBudgetChainPR(w, r)
	if !ok {
		return
	}
	stepID, err := parseID(r, "stepID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid step id")
		return
	}
	step := findBudgetStep(pr.Recommendation, stepID)
	if step == nil {
		writeError(w, http.StatusNotFound, "unknown budget step")
		return
	}
	if step.IsBase {
		// The base step is governed by the budget unit — reuse the existing fan-out.
		h.notifyBudgetApprovers(r, pr)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	h.notifyBudgetStepApprover(r, pr, step)
	w.WriteHeader(http.StatusNoContent)
}

// roleForAssignableType maps an assignable card type (legal/security) to the role
// that designates its team. Reports false for the budget card (no assignee).
func roleForAssignableType(approvalType string) (string, bool) {
	switch approvalType {
	case model.RecApprovalLegal:
		return model.RoleLegal, true
	case model.RecApprovalSecurity:
		return model.RoleSecurity, true
	}
	return "", false
}

// --- comment documents ---

// loadRecComment loads the PR (viewable) and one of its recommendation comments.
func (h *PurchaseRequestsHandler) loadRecComment(w http.ResponseWriter, r *http.Request) (*repository.PurchaseRequest, *repository.RecComment, bool) {
	pr, ok := h.loadViewablePR(w, r)
	if !ok {
		return nil, nil, false
	}
	commentID, err := parseID(r, "commentID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid comment id")
		return nil, nil, false
	}
	c, err := h.Repo.GetRecComment(r.Context(), pr.ID, commentID)
	if err != nil {
		writeError(w, http.StatusNotFound, "comment not found")
		return nil, nil, false
	}
	return pr, c, true
}

func (h *PurchaseRequestsHandler) UploadRecCommentDocument(w http.ResponseWriter, r *http.Request) {
	pr, c, ok := h.loadRecComment(w, r)
	if !ok {
		return
	}
	user := middleware.UserFromCtx(r.Context())
	if c.AuthorID != user.ID {
		writeError(w, http.StatusForbidden, "only the comment's author can attach documents")
		return
	}
	doc, ok := saveUploadedDoc(w, r, h.Repo, h.Storage, pr.ID, model.OwnerRecommendationComment, c.ID, allowedExtensions)
	if !ok {
		return
	}
	writeJSON(w, http.StatusCreated, doc)
}

func (h *PurchaseRequestsHandler) DownloadRecCommentDocument(w http.ResponseWriter, r *http.Request) {
	_, c, ok := h.loadRecComment(w, r)
	if !ok {
		return
	}
	docID, err := parseID(r, "docID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid document id")
		return
	}
	downloadOwnedDoc(w, r, h.Repo, h.Storage, model.OwnerRecommendationComment, c.ID, docID)
}

func (h *PurchaseRequestsHandler) DeleteRecCommentDocument(w http.ResponseWriter, r *http.Request) {
	_, c, ok := h.loadRecComment(w, r)
	if !ok {
		return
	}
	user := middleware.UserFromCtx(r.Context())
	if c.AuthorID != user.ID {
		writeError(w, http.StatusForbidden, "only the comment's author can remove its documents")
		return
	}
	docID, err := parseID(r, "docID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid document id")
		return
	}
	deleteOwnedDoc(w, r, h.Repo, h.Storage, model.OwnerRecommendationComment, c.ID, docID)
}
