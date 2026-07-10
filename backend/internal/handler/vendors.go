package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/cs/purchasing-app/internal/middleware"
	"github.com/cs/purchasing-app/internal/repository"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"
)

type VendorsHandler struct {
	Repo *repository.Repository
	Log  zerolog.Logger
}

type vendorInput struct {
	Name        string `json:"name"`
	ContactName string `json:"contact_name"`
	Email       string `json:"email"`
	Phone       string `json:"phone"`
	Notes       string `json:"notes"`
	IsActive    bool   `json:"is_active"`
	TaxID       string `json:"tax_id"`
	AddressLine string `json:"address_line"`
	City        string `json:"city"`
	PostalCode  string `json:"postal_code"`
	Country     string `json:"country"`
	Website     string `json:"website"`
	Registered  bool   `json:"registered"`
}

func (in vendorInput) toRepo() repository.VendorInput {
	return repository.VendorInput{
		Name:        strings.TrimSpace(in.Name),
		ContactName: strings.TrimSpace(in.ContactName),
		Email:       strings.TrimSpace(in.Email),
		Phone:       strings.TrimSpace(in.Phone),
		Notes:       in.Notes,
		IsActive:    in.IsActive,
		TaxID:       strings.TrimSpace(in.TaxID),
		AddressLine: strings.TrimSpace(in.AddressLine),
		City:        strings.TrimSpace(in.City),
		PostalCode:  strings.TrimSpace(in.PostalCode),
		Country:     strings.TrimSpace(in.Country),
		Website:     strings.TrimSpace(in.Website),
		Registered:  in.Registered,
	}
}

// Lookup returns active vendors as minimal summaries for the purchase-request
// "proposed supplier" dropdown. Any authenticated user may call it (requesters
// are usually staff, who cannot reach the finance-gated full vendor list).
func (h *VendorsHandler) Lookup(w http.ResponseWriter, r *http.Request) {
	vendors, err := h.Repo.ListVendorsLookup(r.Context())
	if err != nil {
		reqLog(r).Error().Err(err).Msg("lookup vendors")
		writeError(w, http.StatusInternalServerError, "failed to list vendors")
		return
	}
	writeJSON(w, http.StatusOK, vendors)
}

func (h *VendorsHandler) List(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasFinanceAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "finance access required")
		return
	}
	vendors, err := h.Repo.ListVendors(r.Context())
	if err != nil {
		reqLog(r).Error().Err(err).Msg("list vendors")
		writeError(w, http.StatusInternalServerError, "failed to list vendors")
		return
	}
	writeJSON(w, http.StatusOK, vendors)
}

func (h *VendorsHandler) Create(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasFinanceAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "finance access required")
		return
	}
	var in vendorInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	repoIn := in.toRepo()
	if repoIn.Name == "" {
		writeError(w, http.StatusBadRequest, "vendor name is required")
		return
	}
	user := middleware.UserFromCtx(r.Context())
	v, err := h.Repo.CreateVendor(r.Context(), repoIn, user.ID)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("create vendor")
		writeError(w, http.StatusInternalServerError, "failed to create vendor")
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (h *VendorsHandler) Get(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasFinanceAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "finance access required")
		return
	}
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid vendor id")
		return
	}
	v, err := h.Repo.GetVendor(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "vendor not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load vendor")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *VendorsHandler) Update(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasVendorAdmin(r.Context()) {
		writeError(w, http.StatusForbidden, "admin or finance_admin access required")
		return
	}
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid vendor id")
		return
	}
	var in vendorInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	repoIn := in.toRepo()
	if repoIn.Name == "" {
		writeError(w, http.StatusBadRequest, "vendor name is required")
		return
	}
	if err := h.Repo.UpdateVendor(r.Context(), id, repoIn); err != nil {
		reqLog(r).Error().Err(err).Msg("update vendor")
		writeError(w, http.StatusInternalServerError, "failed to update vendor")
		return
	}
	v, err := h.Repo.GetVendor(r.Context(), id)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("reload vendor after mutation")
		writeError(w, http.StatusInternalServerError, "failed to reload vendor")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// Usage returns a "where used" summary (counts of related quotations,
// contracts, GRNs and invoices) for a vendor. Management page only.
func (h *VendorsHandler) Usage(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasVendorAdmin(r.Context()) {
		writeError(w, http.StatusForbidden, "admin or finance_admin access required")
		return
	}
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid vendor id")
		return
	}
	usage, err := h.Repo.GetVendorUsage(r.Context(), id)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("vendor usage")
		writeError(w, http.StatusInternalServerError, "failed to load vendor usage")
		return
	}
	writeJSON(w, http.StatusOK, usage)
}
