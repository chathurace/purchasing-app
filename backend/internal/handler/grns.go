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

type GRNsHandler struct {
	Repo    *repository.Repository
	Storage storage.Store
	Log     zerolog.Logger
}

type grnItemInput struct {
	Description string  `json:"description"`
	Quantity    float64 `json:"quantity"`
}

type grnInput struct {
	ReceivedDate string         `json:"received_date"`
	ReceivedBy   string         `json:"received_by"`
	Note         string         `json:"note"`
	Items        []grnItemInput `json:"items"`
}

func (in grnInput) toRepo() repository.GRNInput {
	items := make([]repository.GRNItem, 0, len(in.Items))
	for _, it := range in.Items {
		desc := strings.TrimSpace(it.Description)
		if desc == "" {
			continue
		}
		items = append(items, repository.GRNItem{Description: desc, Quantity: it.Quantity})
	}
	return repository.GRNInput{
		ReceivedDate: strings.TrimSpace(in.ReceivedDate),
		ReceivedBy:   strings.TrimSpace(in.ReceivedBy),
		Note:         in.Note,
		Items:        items,
	}
}

// Create records a GRN against a signed contract.
func (h *GRNsHandler) Create(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasFinanceAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "finance access required")
		return
	}
	contractID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid contract id")
		return
	}
	var in grnInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	user := middleware.UserFromCtx(r.Context())
	g, err := h.Repo.CreateGRN(r.Context(), contractID, in.toRepo(), user.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "contract not found")
			return
		}
		if errors.Is(err, repository.ErrInvalidState) {
			writeError(w, http.StatusConflict, "a GRN can only be recorded against a signed contract")
			return
		}
		h.Log.Error().Err(err).Msg("create grn")
		writeError(w, http.StatusInternalServerError, "failed to create GRN")
		return
	}
	writeJSON(w, http.StatusCreated, g)
}

// ListForContract returns the GRNs of one contract.
func (h *GRNsHandler) ListForContract(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasFinanceAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "finance access required")
		return
	}
	contractID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid contract id")
		return
	}
	grns, err := h.Repo.ListGRNs(r.Context(), &contractID)
	if err != nil {
		h.Log.Error().Err(err).Msg("list grns for contract")
		writeError(w, http.StatusInternalServerError, "failed to list GRNs")
		return
	}
	writeJSON(w, http.StatusOK, grns)
}

func (h *GRNsHandler) List(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasFinanceAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "finance access required")
		return
	}
	grns, err := h.Repo.ListGRNs(r.Context(), nil)
	if err != nil {
		h.Log.Error().Err(err).Msg("list grns")
		writeError(w, http.StatusInternalServerError, "failed to list GRNs")
		return
	}
	writeJSON(w, http.StatusOK, grns)
}

func (h *GRNsHandler) Get(w http.ResponseWriter, r *http.Request) {
	g, ok := h.load(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (h *GRNsHandler) Update(w http.ResponseWriter, r *http.Request) {
	g, ok := h.load(w, r)
	if !ok {
		return
	}
	var in grnInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.Repo.UpdateGRN(r.Context(), g.ID, in.toRepo()); err != nil {
		h.Log.Error().Err(err).Msg("update grn")
		writeError(w, http.StatusInternalServerError, "failed to update GRN")
		return
	}
	updated, err := h.Repo.GetGRN(r.Context(), g.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reload GRN")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *GRNsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	g, ok := h.load(w, r)
	if !ok {
		return
	}
	paths, err := h.Repo.DeleteGRN(r.Context(), g.ID)
	if err != nil {
		h.Log.Error().Err(err).Msg("delete grn")
		writeError(w, http.StatusInternalServerError, "failed to delete GRN")
		return
	}
	for _, p := range paths {
		_ = h.Storage.Delete(p)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *GRNsHandler) UploadDocument(w http.ResponseWriter, r *http.Request) {
	g, ok := h.load(w, r)
	if !ok {
		return
	}
	doc, ok := saveUploadedDoc(w, r, h.Repo, h.Storage, h.Log, g.PurchaseRequestID, model.OwnerGRN, g.ID, allowedExtensions)
	if !ok {
		return
	}
	writeJSON(w, http.StatusCreated, doc)
}

func (h *GRNsHandler) DownloadDocument(w http.ResponseWriter, r *http.Request) {
	g, ok := h.load(w, r)
	if !ok {
		return
	}
	docID, err := parseID(r, "docID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid document id")
		return
	}
	downloadOwnedDoc(w, r, h.Repo, h.Storage, h.Log, model.OwnerGRN, g.ID, docID)
}

func (h *GRNsHandler) DeleteDocument(w http.ResponseWriter, r *http.Request) {
	g, ok := h.load(w, r)
	if !ok {
		return
	}
	docID, err := parseID(r, "docID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid document id")
		return
	}
	deleteOwnedDoc(w, r, h.Repo, h.Storage, model.OwnerGRN, g.ID, docID)
}

// load fetches the GRN for finance-level actions, enforcing finance access.
func (h *GRNsHandler) load(w http.ResponseWriter, r *http.Request) (*repository.GRN, bool) {
	if !middleware.HasFinanceAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "finance access required")
		return nil, false
	}
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid GRN id")
		return nil, false
	}
	g, err := h.Repo.GetGRN(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "GRN not found")
			return nil, false
		}
		h.Log.Error().Err(err).Msg("get grn")
		writeError(w, http.StatusInternalServerError, "failed to load GRN")
		return nil, false
	}
	return g, true
}
