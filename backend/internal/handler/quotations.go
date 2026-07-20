package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/cs/purchasing-app/internal/email"
	"github.com/cs/purchasing-app/internal/middleware"
	"github.com/cs/purchasing-app/internal/model"
	"github.com/cs/purchasing-app/internal/repository"
	"github.com/cs/purchasing-app/internal/storage"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"
)

type QuotationsHandler struct {
	Repo       *repository.Repository
	Storage    storage.Store
	Mailer     email.Mailer
	AppBaseURL string
	Log        zerolog.Logger
}

type quotationItemInput struct {
	Description string  `json:"description"`
	Quantity    float64 `json:"quantity"`
	UnitPrice   float64 `json:"unit_price"`
}

type quotationInput struct {
	VendorID    int64                `json:"vendor_id"`
	TotalAmount float64              `json:"total_amount"`
	Currency    string               `json:"currency"`
	ValidUntil  *string              `json:"valid_until"`
	Notes       string               `json:"notes"`
	Items       []quotationItemInput `json:"items"`
}

func (in quotationInput) toRepo() repository.QuotationInput {
	out := repository.QuotationInput{
		VendorID:    in.VendorID,
		TotalAmount: in.TotalAmount,
		Currency:    strings.TrimSpace(in.Currency),
		ValidUntil:  in.ValidUntil,
		Notes:       in.Notes,
	}
	if out.Currency == "" {
		out.Currency = "USD"
	}
	for _, it := range in.Items {
		desc := strings.TrimSpace(it.Description)
		if desc == "" {
			continue
		}
		out.Items = append(out.Items, repository.QuotationItem{
			Description: desc,
			Quantity:    it.Quantity,
			UnitPrice:   it.UnitPrice,
		})
	}
	return out
}

// ListForPR returns the quotations associated with a purchase request.
func (h *QuotationsHandler) ListForPR(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasProcurementAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "procurement access required")
		return
	}
	prID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request id")
		return
	}
	quotes, err := h.Repo.ListQuotations(r.Context(), &prID)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("list quotations for pr")
		writeError(w, http.StatusInternalServerError, "failed to list quotations")
		return
	}
	writeJSON(w, http.StatusOK, quotes)
}

// Create associates a new quotation with a purchase request.
func (h *QuotationsHandler) Create(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasProcurementAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "procurement access required")
		return
	}
	prID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request id")
		return
	}
	pr, err := h.Repo.GetPurchaseRequest(r.Context(), prID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "request not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load request")
		return
	}
	// Procurement work is gated behind team lead approval.
	if !model.IsTeamLeadApproved(pr.TeamLeadStatus) {
		writeError(w, http.StatusConflict, "purchase request is awaiting team lead approval")
		return
	}
	// ...and behind assignment: the PR must be assigned and the caller must be the
	// assignee, a collaborator, or a procurement_admin/admin.
	if code, msg, ok := assignmentWorkGate(r, pr); !ok {
		writeError(w, code, msg)
		return
	}
	var in quotationInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if in.VendorID == 0 {
		writeError(w, http.StatusBadRequest, "a vendor is required")
		return
	}
	user := middleware.UserFromCtx(r.Context())
	q, err := h.Repo.CreateQuotation(r.Context(), prID, in.toRepo(), user.ID)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("create quotation")
		writeError(w, http.StatusInternalServerError, "failed to create quotation")
		return
	}
	recordProcessEvent(r, h.Repo, q.PurchaseRequestID, model.ProcessAddQuotation, "")
	ensureCollaborator(r, h.Repo, q.PurchaseRequestID)
	h.notifyQuotationAdded(r, pr)
	writeJSON(w, http.StatusCreated, q)
}

// notifyQuotationAdded tells the Procurement team + the PR's assignee/collaborators
// that a quotation was added. Best-effort.
func (h *QuotationsHandler) notifyQuotationAdded(r *http.Request, pr *repository.PurchaseRequest) {
	title := prTitleOrRef(pr)
	subject := fmt.Sprintf("Quotation added: %s", title)
	body := fmt.Sprintf(
		"Hi,\n\n%s added a quotation to purchase request %s — \"%s\".\n\nView it here:\n%s\n",
		actorName(r), prDisplayRef(pr), title, prURLWith(h.AppBaseURL, pr))
	notifyPRActivity(r.Context(), h.Repo, h.Mailer, h.Log, pr, actorEmail(r), subject, body)
}

func (h *QuotationsHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var (
		quotes []*repository.Quotation
		err    error
	)
	if middleware.HasProcurementAccess(ctx) {
		quotes, err = h.Repo.ListQuotations(ctx, nil)
	} else {
		// Approvers get a read-only view scoped to the PRs they approve.
		user := middleware.UserFromCtx(ctx)
		quotes, err = h.Repo.ListQuotationsForApprover(ctx, user.ID,
			middleware.HasRole(ctx, model.RoleLegal), middleware.HasRole(ctx, model.RoleSecurity))
	}
	if err != nil {
		reqLog(r).Error().Err(err).Msg("list quotations")
		writeError(w, http.StatusInternalServerError, "failed to list quotations")
		return
	}
	writeJSON(w, http.StatusOK, quotes)
}

func (h *QuotationsHandler) Get(w http.ResponseWriter, r *http.Request) {
	q, ok := h.loadViewable(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, q)
}

func (h *QuotationsHandler) Update(w http.ResponseWriter, r *http.Request) {
	q, ok := h.load(w, r)
	if !ok {
		return
	}
	var in quotationInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if in.VendorID == 0 {
		writeError(w, http.StatusBadRequest, "a vendor is required")
		return
	}
	if err := h.Repo.UpdateQuotation(r.Context(), q.ID, in.toRepo()); err != nil {
		reqLog(r).Error().Err(err).Msg("update quotation")
		writeError(w, http.StatusInternalServerError, "failed to update quotation")
		return
	}
	recordProcessEvent(r, h.Repo, q.PurchaseRequestID, model.ProcessUpdateQuotation, "")
	ensureCollaborator(r, h.Repo, q.PurchaseRequestID)
	updated, err := h.Repo.GetQuotation(r.Context(), q.ID)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("reload quotation after mutation")
		writeError(w, http.StatusInternalServerError, "failed to reload quotation")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// Select marks the quotation as the chosen vendor response.
func (h *QuotationsHandler) Select(w http.ResponseWriter, r *http.Request) {
	q, ok := h.load(w, r)
	if !ok {
		return
	}
	if err := h.Repo.SelectQuotation(r.Context(), q.ID); err != nil {
		if errors.Is(err, repository.ErrInvalidState) {
			writeError(w, http.StatusConflict, "the procurement recommendation must be fully approved before selecting a quotation")
			return
		}
		reqLog(r).Error().Err(err).Msg("select quotation")
		writeError(w, http.StatusInternalServerError, "failed to select quotation")
		return
	}
	recordProcessEvent(r, h.Repo, q.PurchaseRequestID, model.ProcessSelectQuotation, "")
	ensureCollaborator(r, h.Repo, q.PurchaseRequestID)
	updated, err := h.Repo.GetQuotation(r.Context(), q.ID)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("reload quotation after mutation")
		writeError(w, http.StatusInternalServerError, "failed to reload quotation")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *QuotationsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	q, ok := h.load(w, r)
	if !ok {
		return
	}
	paths, err := h.Repo.DeleteQuotation(r.Context(), q.ID)
	if err != nil {
		if errors.Is(err, repository.ErrHasChildren) {
			writeError(w, http.StatusConflict, "cannot delete a quotation that has a contract")
			return
		}
		reqLog(r).Error().Err(err).Msg("delete quotation")
		writeError(w, http.StatusInternalServerError, "failed to delete quotation")
		return
	}
	for _, p := range paths {
		_ = h.Storage.Delete(p)
	}
	recordProcessEvent(r, h.Repo, q.PurchaseRequestID, model.ProcessDeleteQuotation, "")
	w.WriteHeader(http.StatusNoContent)
}

// --- documents ---

func (h *QuotationsHandler) UploadDocument(w http.ResponseWriter, r *http.Request) {
	q, ok := h.load(w, r)
	if !ok {
		return
	}
	doc, ok := saveUploadedDoc(w, r, h.Repo, h.Storage, q.PurchaseRequestID, model.OwnerQuotation, q.ID, allowedExtensions)
	if !ok {
		return
	}
	writeJSON(w, http.StatusCreated, doc)
}

func (h *QuotationsHandler) DownloadDocument(w http.ResponseWriter, r *http.Request) {
	q, ok := h.loadViewable(w, r)
	if !ok {
		return
	}
	docID, err := parseID(r, "docID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid document id")
		return
	}
	downloadOwnedDoc(w, r, h.Repo, h.Storage, model.OwnerQuotation, q.ID, docID)
}

func (h *QuotationsHandler) DeleteDocument(w http.ResponseWriter, r *http.Request) {
	q, ok := h.load(w, r)
	if !ok {
		return
	}
	docID, err := parseID(r, "docID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid document id")
		return
	}
	deleteOwnedDoc(w, r, h.Repo, h.Storage, model.OwnerQuotation, q.ID, docID)
}

// --- primary quotation PDF (single; replaceable + removable) ---

// UploadQuotationPDF attaches (or replaces) the quotation's single primary PDF.
// On replace, the previously attached PDF is deleted.
func (h *QuotationsHandler) UploadQuotationPDF(w http.ResponseWriter, r *http.Request) {
	q, ok := h.load(w, r)
	if !ok {
		return
	}
	doc, ok := saveUploadedDoc(w, r, h.Repo, h.Storage, q.PurchaseRequestID, model.OwnerQuotation, q.ID, pdfOnly)
	if !ok {
		return
	}
	prevID, err := h.Repo.SetQuotationDocument(r.Context(), q.ID, doc.ID)
	if err != nil {
		_ = h.Repo.DeleteOwnedDocument(r.Context(), model.OwnerQuotation, q.ID, doc.ID)
		_ = h.Storage.Delete(doc.StoredPath)
		reqLog(r).Error().Err(err).Msg("set quotation document")
		writeError(w, http.StatusInternalServerError, "failed to attach quotation PDF")
		return
	}
	// Clean up the replaced PDF, if any.
	if prevID != nil {
		if old, err := h.Repo.GetOwnedDocument(r.Context(), model.OwnerQuotation, q.ID, *prevID); err == nil {
			_ = h.Repo.DeleteOwnedDocument(r.Context(), model.OwnerQuotation, q.ID, *prevID)
			_ = h.Storage.Delete(old.StoredPath)
		}
	}
	updated, err := h.Repo.GetQuotation(r.Context(), q.ID)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("reload quotation after mutation")
		writeError(w, http.StatusInternalServerError, "failed to reload quotation")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// DeleteQuotationPDF removes the quotation's primary PDF.
func (h *QuotationsHandler) DeleteQuotationPDF(w http.ResponseWriter, r *http.Request) {
	q, ok := h.load(w, r)
	if !ok {
		return
	}
	path, err := h.Repo.ClearQuotationDocument(r.Context(), q.ID)
	if err != nil {
		if errors.Is(err, repository.ErrInvalidState) {
			writeError(w, http.StatusConflict, "no quotation PDF is attached")
			return
		}
		reqLog(r).Error().Err(err).Msg("clear quotation document")
		writeError(w, http.StatusInternalServerError, "failed to remove quotation PDF")
		return
	}
	if path != "" {
		_ = h.Storage.Delete(path)
	}
	updated, err := h.Repo.GetQuotation(r.Context(), q.ID)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("reload quotation after mutation")
		writeError(w, http.StatusInternalServerError, "failed to reload quotation")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// load fetches a quotation for a mutating action — procurement access required.
func (h *QuotationsHandler) load(w http.ResponseWriter, r *http.Request) (*repository.Quotation, bool) {
	if !middleware.HasProcurementAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "procurement access required")
		return nil, false
	}
	return h.fetch(w, r)
}

// loadViewable fetches a quotation for a read action: procurement/admin, or an
// approver on the quotation's PR (read-only). Mutations must use load.
func (h *QuotationsHandler) loadViewable(w http.ResponseWriter, r *http.Request) (*repository.Quotation, bool) {
	q, ok := h.fetch(w, r)
	if !ok {
		return nil, false
	}
	ctx := r.Context()
	if middleware.HasProcurementAccess(ctx) {
		return q, true
	}
	user := middleware.UserFromCtx(ctx)
	if user != nil {
		ok, err := h.Repo.IsApproverForPR(ctx, q.PurchaseRequestID, user.ID,
			middleware.HasRole(ctx, model.RoleLegal), middleware.HasRole(ctx, model.RoleSecurity))
		if err != nil {
			reqLog(r).Error().Err(err).Msg("check quotation approver access")
			writeError(w, http.StatusInternalServerError, "failed to load quotation")
			return nil, false
		}
		if ok {
			return q, true
		}
	}
	writeError(w, http.StatusForbidden, "not authorized to view this quotation")
	return nil, false
}

// fetch loads a quotation by URL id without any access check.
func (h *QuotationsHandler) fetch(w http.ResponseWriter, r *http.Request) (*repository.Quotation, bool) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid quotation id")
		return nil, false
	}
	q, err := h.Repo.GetQuotation(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "quotation not found")
			return nil, false
		}
		reqLog(r).Error().Err(err).Msg("get quotation")
		writeError(w, http.StatusInternalServerError, "failed to load quotation")
		return nil, false
	}
	return q, true
}
