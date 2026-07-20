package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/cs/purchasing-app/internal/middleware"
	"github.com/cs/purchasing-app/internal/model"
	"github.com/cs/purchasing-app/internal/repository"
	"github.com/cs/purchasing-app/internal/storage"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"
)

type ContractsHandler struct {
	Repo    *repository.Repository
	Storage storage.Store
	Log     zerolog.Logger
}

type contractInput struct {
	Title       string  `json:"title"`
	TotalAmount float64 `json:"total_amount"`
	Currency    string  `json:"currency"`
	Terms       string  `json:"terms"`
}

func (in contractInput) toRepo() repository.ContractInput {
	out := repository.ContractInput{
		Title:       strings.TrimSpace(in.Title),
		TotalAmount: in.TotalAmount,
		Currency:    strings.TrimSpace(in.Currency),
		Terms:       in.Terms,
	}
	if out.Currency == "" {
		out.Currency = "USD"
	}
	return out
}

func (h *ContractsHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var (
		contracts []*repository.Contract
		err       error
	)
	if middleware.HasProcurementAccess(ctx) {
		contracts, err = h.Repo.ListContracts(ctx, nil)
	} else {
		// Approvers (incl. legal/security) get a read-only view scoped to the PRs
		// they approve, rather than the full contract set.
		user := middleware.UserFromCtx(ctx)
		contracts, err = h.Repo.ListContractsForApprover(ctx, user.ID,
			middleware.HasRole(ctx, model.RoleLegal), middleware.HasRole(ctx, model.RoleSecurity))
	}
	if err != nil {
		reqLog(r).Error().Err(err).Msg("list contracts")
		writeError(w, http.StatusInternalServerError, "failed to list contracts")
		return
	}
	writeJSON(w, http.StatusOK, contracts)
}

func (h *ContractsHandler) Get(w http.ResponseWriter, r *http.Request) {
	c, ok := h.loadViewable(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *ContractsHandler) Update(w http.ResponseWriter, r *http.Request) {
	c, ok := h.load(w, r)
	if !ok {
		return
	}
	if c.Status == model.ContractSigned {
		writeError(w, http.StatusConflict, "a signed contract can no longer be edited")
		return
	}
	var in contractInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.Repo.UpdateContract(r.Context(), c.ID, in.toRepo()); err != nil {
		reqLog(r).Error().Err(err).Msg("update contract")
		writeError(w, http.StatusInternalServerError, "failed to update contract")
		return
	}
	recordProcessEvent(r, h.Repo, c.PurchaseRequestID, model.ProcessUpdateContract, "")
	updated, err := h.Repo.GetContract(r.Context(), c.ID)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("reload contract after mutation")
		writeError(w, http.StatusInternalServerError, "failed to reload contract")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// UploadSignedDocument attaches (or replaces) the signed contract PDF and marks
// the contract signed. Allowed from draft or signed; on replace the previous
// signed PDF is deleted. An optional "notes" form field is stored on the doc.
func (h *ContractsHandler) UploadSignedDocument(w http.ResponseWriter, r *http.Request) {
	c, ok := h.load(w, r)
	if !ok {
		return
	}
	doc, ok := saveUploadedDoc(w, r, h.Repo, h.Storage, c.PurchaseRequestID, model.OwnerContract, c.ID, pdfOnly)
	if !ok {
		return
	}
	prevSignedID, err := h.Repo.SetContractSigned(r.Context(), c.ID, doc.ID)
	if err != nil {
		_ = h.Repo.DeleteOwnedDocument(r.Context(), model.OwnerContract, c.ID, doc.ID)
		_ = h.Storage.Delete(doc.StoredPath)
		if errors.Is(err, repository.ErrInvalidState) {
			writeError(w, http.StatusConflict, "this contract cannot be signed in its current state")
			return
		}
		reqLog(r).Error().Err(err).Msg("sign contract")
		writeError(w, http.StatusInternalServerError, "failed to sign contract")
		return
	}
	// Clean up the replaced signed PDF, if any.
	if prevSignedID != nil {
		if old, err := h.Repo.GetOwnedDocument(r.Context(), model.OwnerContract, c.ID, *prevSignedID); err == nil {
			_ = h.Repo.DeleteOwnedDocument(r.Context(), model.OwnerContract, c.ID, *prevSignedID)
			_ = h.Storage.Delete(old.StoredPath)
		}
	}
	recordProcessEvent(r, h.Repo, c.PurchaseRequestID, model.ProcessSignContract, model.QualifierSign)
	updated, err := h.Repo.GetContract(r.Context(), c.ID)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("reload contract after mutation")
		writeError(w, http.StatusInternalServerError, "failed to reload contract")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// DeleteSignedDocument removes the signed PDF and reverts the contract to draft.
func (h *ContractsHandler) DeleteSignedDocument(w http.ResponseWriter, r *http.Request) {
	c, ok := h.load(w, r)
	if !ok {
		return
	}
	path, err := h.Repo.ClearContractSigned(r.Context(), c.ID)
	if err != nil {
		if errors.Is(err, repository.ErrInvalidState) {
			writeError(w, http.StatusConflict, "cannot remove the signed PDF: none set, or the contract already has GRNs/invoices")
			return
		}
		reqLog(r).Error().Err(err).Msg("clear signed contract")
		writeError(w, http.StatusInternalServerError, "failed to remove signed document")
		return
	}
	if path != "" {
		_ = h.Storage.Delete(path)
	}
	recordProcessEvent(r, h.Repo, c.PurchaseRequestID, model.ProcessSignContract, model.QualifierUnsign)
	updated, err := h.Repo.GetContract(r.Context(), c.ID)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("reload contract after mutation")
		writeError(w, http.StatusInternalServerError, "failed to reload contract")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// --- draft contract documents (PDF + notes each) ---

func (h *ContractsHandler) UploadDocument(w http.ResponseWriter, r *http.Request) {
	c, ok := h.load(w, r)
	if !ok {
		return
	}
	doc, ok := saveUploadedDoc(w, r, h.Repo, h.Storage, c.PurchaseRequestID, model.OwnerContract, c.ID, pdfOnly)
	if !ok {
		return
	}
	writeJSON(w, http.StatusCreated, doc)
}

// UpdateDocument edits a contract document's notes (draft or signed PDF).
func (h *ContractsHandler) UpdateDocument(w http.ResponseWriter, r *http.Request) {
	c, ok := h.load(w, r)
	if !ok {
		return
	}
	docID, err := parseID(r, "docID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid document id")
		return
	}
	var in struct {
		Notes string `json:"notes"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.Repo.UpdateOwnedDocumentNotes(r.Context(), model.OwnerContract, c.ID, docID, strings.TrimSpace(in.Notes)); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "document not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to update notes")
		return
	}
	updated, err := h.Repo.GetContract(r.Context(), c.ID)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("reload contract after mutation")
		writeError(w, http.StatusInternalServerError, "failed to reload contract")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *ContractsHandler) DownloadDocument(w http.ResponseWriter, r *http.Request) {
	c, ok := h.loadViewable(w, r)
	if !ok {
		return
	}
	docID, err := parseID(r, "docID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid document id")
		return
	}
	downloadOwnedDoc(w, r, h.Repo, h.Storage, model.OwnerContract, c.ID, docID)
}

func (h *ContractsHandler) DeleteDocument(w http.ResponseWriter, r *http.Request) {
	c, ok := h.load(w, r)
	if !ok {
		return
	}
	docID, err := parseID(r, "docID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid document id")
		return
	}
	deleteOwnedDoc(w, r, h.Repo, h.Storage, model.OwnerContract, c.ID, docID)
}

// load fetches the contract for procurement-level actions (create, edit, sign,
// supporting documents).
func (h *ContractsHandler) load(w http.ResponseWriter, r *http.Request) (*repository.Contract, bool) {
	if !middleware.HasProcurementAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "procurement access required")
		return nil, false
	}
	return h.fetch(w, r)
}

// loadViewable fetches the contract for read access: procurement/admin, or an
// approver (legal/security card actor or budget owner) on the contract's PR —
// read-only and scoped to PRs the caller is actually involved with. Mutations
// use load, which is procurement-only.
func (h *ContractsHandler) loadViewable(w http.ResponseWriter, r *http.Request) (*repository.Contract, bool) {
	c, ok := h.fetch(w, r)
	if !ok {
		return nil, false
	}
	ctx := r.Context()
	if middleware.HasProcurementAccess(ctx) {
		return c, true
	}
	user := middleware.UserFromCtx(ctx)
	if user != nil {
		ok, err := h.Repo.IsApproverForPR(ctx, c.PurchaseRequestID, user.ID,
			middleware.HasRole(ctx, model.RoleLegal), middleware.HasRole(ctx, model.RoleSecurity))
		if err != nil {
			reqLog(r).Error().Err(err).Msg("check contract approver access")
			writeError(w, http.StatusInternalServerError, "failed to load contract")
			return nil, false
		}
		if ok {
			return c, true
		}
	}
	writeError(w, http.StatusForbidden, "contract access required")
	return nil, false
}

func (h *ContractsHandler) fetch(w http.ResponseWriter, r *http.Request) (*repository.Contract, bool) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid contract id")
		return nil, false
	}
	c, err := h.Repo.GetContract(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "contract not found")
			return nil, false
		}
		reqLog(r).Error().Err(err).Msg("get contract")
		writeError(w, http.StatusInternalServerError, "failed to load contract")
		return nil, false
	}
	return c, true
}
