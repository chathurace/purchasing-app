package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/cs/purchasing-app/internal/middleware"
	"github.com/cs/purchasing-app/internal/model"
	"github.com/cs/purchasing-app/internal/repository"
)

// Procurement recommendation endpoints. These hang off a purchase request and
// reuse the PurchaseRequestsHandler so they share its repo, storage and the
// PR view/permission helpers.

type recommendationInput struct {
	VendorID      int64    `json:"vendor_id"`
	Description   string   `json:"description"`
	RequiredTypes []string `json:"required_types"`
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
	if !middleware.HasFinanceAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "finance access required")
		return
	}
	pr, ok := h.load(w, r)
	if !ok {
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
	types, ok := normalizeRequiredTypes(in.RequiredTypes)
	if !ok {
		writeError(w, http.StatusBadRequest, "select at least one approval (budget, legal or security)")
		return
	}
	if in.VendorID == 0 {
		writeError(w, http.StatusBadRequest, "a vendor is required")
		return
	}
	hasQuote, err := h.vendorHasQuotation(r, pr.ID, in.VendorID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to validate vendor")
		return
	}
	if !hasQuote {
		writeError(w, http.StatusBadRequest, "the vendor must have a quotation on this request")
		return
	}
	user := middleware.UserFromCtx(r.Context())
	if _, err := h.Repo.CreateRecommendation(r.Context(), pr.ID, in.VendorID, strings.TrimSpace(in.Description), types, user.ID); err != nil {
		reqLog(r).Error().Err(err).Msg("create recommendation")
		writeError(w, http.StatusInternalServerError, "failed to create recommendation")
		return
	}
	h.reloadPR(w, r, pr.ID)
}

// UpdateRecommendation replaces the recommendation's vendor / description /
// required approvals, resetting every card to pending.
func (h *PurchaseRequestsHandler) UpdateRecommendation(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasFinanceAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "finance access required")
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
	types, ok := normalizeRequiredTypes(in.RequiredTypes)
	if !ok {
		writeError(w, http.StatusBadRequest, "select at least one approval (budget, legal or security)")
		return
	}
	if in.VendorID == 0 {
		writeError(w, http.StatusBadRequest, "a vendor is required")
		return
	}
	hasQuote, err := h.vendorHasQuotation(r, pr.ID, in.VendorID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to validate vendor")
		return
	}
	if !hasQuote {
		writeError(w, http.StatusBadRequest, "the vendor must have a quotation on this request")
		return
	}
	paths, err := h.Repo.UpdateRecommendation(r.Context(), pr.ID, in.VendorID, strings.TrimSpace(in.Description), types)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("update recommendation")
		writeError(w, http.StatusInternalServerError, "failed to update recommendation")
		return
	}
	for _, p := range paths {
		_ = h.Storage.Delete(p)
	}
	h.reloadPR(w, r, pr.ID)
}

// DeleteRecommendation removes a PR's recommendation.
func (h *PurchaseRequestsHandler) DeleteRecommendation(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasFinanceAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "finance access required")
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
	w.WriteHeader(http.StatusNoContent)
}

// CreateRecommendationContract drafts the contract attached to a PR's
// recommendation (the optional contract card). The given description becomes the
// contract terms; the PDF is uploaded separately against the returned contract
// (POST /contracts/{id}/documents). Finance only; one contract per recommendation.
func (h *PurchaseRequestsHandler) CreateRecommendationContract(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasFinanceAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "finance access required")
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
	writeJSON(w, http.StatusCreated, c)
}

// DeleteRecommendationContract removes the contract attached to a PR's
// recommendation (only while it is still a draft) and unlinks it. Finance only.
func (h *PurchaseRequestsHandler) DeleteRecommendationContract(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasFinanceAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "finance access required")
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
	h.reloadPR(w, r, pr.ID)
}

// SetRecommendationRFI sets the RFI description on a PR's recommendation (the
// optional RFI card). Finance only. The PDF attachments are managed separately.
func (h *PurchaseRequestsHandler) SetRecommendationRFI(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasFinanceAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "finance access required")
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
	h.reloadPR(w, r, pr.ID)
}

// DeleteRecommendationRFI clears a PR's RFI: blanks the description and removes
// all RFI attachments. Finance only.
func (h *PurchaseRequestsHandler) DeleteRecommendationRFI(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasFinanceAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "finance access required")
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
	h.reloadPR(w, r, pr.ID)
}

// UploadRecRFIDocument attaches a PDF to a PR's RFI. Finance only.
func (h *PurchaseRequestsHandler) UploadRecRFIDocument(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasFinanceAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "finance access required")
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

// DeleteRecRFIDocument removes an RFI attachment. Finance only.
func (h *PurchaseRequestsHandler) DeleteRecRFIDocument(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasFinanceAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "finance access required")
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
	can, err := h.canActOnRecType(r, pr, approvalType)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check permission")
		return
	}
	if !can {
		writeError(w, http.StatusForbidden, "you cannot act on the "+approvalType+" approval")
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
	can, err := h.canActOnRecType(r, pr, in.ApprovalType)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check permission")
		return
	}
	if !can {
		writeError(w, http.StatusForbidden, "you cannot comment on the "+in.ApprovalType+" approval")
		return
	}
	user := middleware.UserFromCtx(r.Context())
	comment, err := h.Repo.AddRecComment(r.Context(), pr.ID, in.ApprovalType, user.ID, strings.TrimSpace(in.Comment))
	if err != nil {
		reqLog(r).Error().Err(err).Msg("add recommendation comment")
		writeError(w, http.StatusInternalServerError, "failed to add comment")
		return
	}
	writeJSON(w, http.StatusCreated, comment)
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
