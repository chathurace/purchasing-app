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

type CostCentersHandler struct {
	Repo *repository.Repository
	Log  zerolog.Logger
}

type costCenterInput struct {
	Code              string  `json:"code"`
	Name              string  `json:"name"`
	Description       string  `json:"description"`
	PrimaryOwnerID    *int64  `json:"primary_owner_id"`
	Budget            float64 `json:"budget"`
	Currency          string  `json:"currency"`
	IsActive          bool    `json:"is_active"`
	SecondaryOwnerIDs []int64 `json:"secondary_owner_ids"`
}

func (in costCenterInput) toRepo() repository.CostCenterInput {
	// Drop a primary owner that also appears among the secondary owners so a
	// user is never recorded in both roles.
	secondary := in.SecondaryOwnerIDs
	if in.PrimaryOwnerID != nil {
		filtered := secondary[:0:0]
		for _, id := range secondary {
			if id != *in.PrimaryOwnerID {
				filtered = append(filtered, id)
			}
		}
		secondary = filtered
	}
	return repository.CostCenterInput{
		Code:              strings.TrimSpace(in.Code),
		Name:              strings.TrimSpace(in.Name),
		Description:       in.Description,
		PrimaryOwnerID:    in.PrimaryOwnerID,
		Budget:            in.Budget,
		Currency:          strings.TrimSpace(in.Currency),
		IsActive:          in.IsActive,
		SecondaryOwnerIDs: secondary,
	}
}

// Lookup returns active cost centers as summaries for the purchase-request
// dropdown. Any authenticated user may call it.
func (h *CostCentersHandler) Lookup(w http.ResponseWriter, r *http.Request) {
	centers, err := h.Repo.ListActiveCostCenters(r.Context())
	if err != nil {
		reqLog(r).Error().Err(err).Msg("lookup cost centers")
		writeError(w, http.StatusInternalServerError, "failed to list cost centers")
		return
	}
	writeJSON(w, http.StatusOK, centers)
}

func (h *CostCentersHandler) List(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasCostCenterAdmin(r.Context()) {
		writeError(w, http.StatusForbidden, "admin or finance_admin access required")
		return
	}
	centers, err := h.Repo.ListCostCenters(r.Context())
	if err != nil {
		reqLog(r).Error().Err(err).Msg("list cost centers")
		writeError(w, http.StatusInternalServerError, "failed to list cost centers")
		return
	}
	writeJSON(w, http.StatusOK, centers)
}

func (h *CostCentersHandler) Create(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasCostCenterAdmin(r.Context()) {
		writeError(w, http.StatusForbidden, "admin or finance_admin access required")
		return
	}
	var in costCenterInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	repoIn := in.toRepo()
	if repoIn.Name == "" {
		writeError(w, http.StatusBadRequest, "cost center name is required")
		return
	}
	user := middleware.UserFromCtx(r.Context())
	c, err := h.Repo.CreateCostCenter(r.Context(), repoIn, user.ID)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("create cost center")
		writeError(w, http.StatusInternalServerError, "failed to create cost center")
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (h *CostCentersHandler) Get(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasCostCenterAdmin(r.Context()) {
		writeError(w, http.StatusForbidden, "admin or finance_admin access required")
		return
	}
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid cost center id")
		return
	}
	c, err := h.Repo.GetCostCenter(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "cost center not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load cost center")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *CostCentersHandler) Update(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasCostCenterAdmin(r.Context()) {
		writeError(w, http.StatusForbidden, "admin or finance_admin access required")
		return
	}
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid cost center id")
		return
	}
	var in costCenterInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	repoIn := in.toRepo()
	if repoIn.Name == "" {
		writeError(w, http.StatusBadRequest, "cost center name is required")
		return
	}
	if err := h.Repo.UpdateCostCenter(r.Context(), id, repoIn); err != nil {
		reqLog(r).Error().Err(err).Msg("update cost center")
		writeError(w, http.StatusInternalServerError, "failed to update cost center")
		return
	}
	c, err := h.Repo.GetCostCenter(r.Context(), id)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("reload cost center after mutation")
		writeError(w, http.StatusInternalServerError, "failed to reload cost center")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// Usage returns a "where used" summary (count of referencing purchase requests)
// for a cost center. Management page only.
func (h *CostCentersHandler) Usage(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasCostCenterAdmin(r.Context()) {
		writeError(w, http.StatusForbidden, "admin or finance_admin access required")
		return
	}
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid cost center id")
		return
	}
	usage, err := h.Repo.GetCostCenterUsage(r.Context(), id)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("cost center usage")
		writeError(w, http.StatusInternalServerError, "failed to load cost center usage")
		return
	}
	writeJSON(w, http.StatusOK, usage)
}

// Invoices returns the invoices allocated to a cost center, bucketed by status
// (pending/approved/paid) with the cost center's allocated share totalled per
// currency. Management page only.
func (h *CostCentersHandler) Invoices(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasCostCenterAdmin(r.Context()) {
		writeError(w, http.StatusForbidden, "admin or finance_admin access required")
		return
	}
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid cost center id")
		return
	}
	summary, err := h.Repo.GetCostCenterInvoiceSummary(r.Context(), id)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("cost center invoices")
		writeError(w, http.StatusInternalServerError, "failed to load cost center invoices")
		return
	}
	writeJSON(w, http.StatusOK, summary)
}
