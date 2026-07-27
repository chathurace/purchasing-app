package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/cs/purchasing-app/internal/middleware"
	"github.com/cs/purchasing-app/internal/model"
	"github.com/cs/purchasing-app/internal/repository"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"
)

type BusinessUnitsHandler struct {
	Repo *repository.Repository
	Log  zerolog.Logger
}

type businessUnitInput struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	IsActive    bool    `json:"is_active"`
	ApproverIDs []int64 `json:"approver_ids"`
}

// toRepo validates and builds the repository input. It returns the input and a
// user-facing error message (empty on success).
func (in businessUnitInput) toRepo() (repository.BusinessUnitInput, string) {
	if strings.TrimSpace(in.Name) == "" {
		return repository.BusinessUnitInput{}, "business unit name is required"
	}
	// Dedupe approver ids, preserving order.
	seen := map[int64]bool{}
	ids := make([]int64, 0, len(in.ApproverIDs))
	for _, id := range in.ApproverIDs {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return repository.BusinessUnitInput{}, "at least one approver is required"
	}
	return repository.BusinessUnitInput{
		Name:        strings.TrimSpace(in.Name),
		Description: in.Description,
		IsActive:    in.IsActive,
		ApproverIDs: ids,
	}, ""
}

// Lookup returns active business units as summaries for the purchase-request
// dropdown. Any authenticated user may call it.
func (h *BusinessUnitsHandler) Lookup(w http.ResponseWriter, r *http.Request) {
	units, err := h.Repo.ListActiveBusinessUnits(r.Context())
	if err != nil {
		reqLog(r).Error().Err(err).Msg("lookup business units")
		writeError(w, http.StatusInternalServerError, "failed to list business units")
		return
	}
	writeJSON(w, http.StatusOK, units)
}

// Approvers returns the flat approver list of a business unit — used to populate
// the budget-approver dropdown on the requisition form. Any authenticated user
// may call it.
func (h *BusinessUnitsHandler) Approvers(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid business unit id")
		return
	}
	approvers, err := h.Repo.BusinessUnitApprovers(r.Context(), id)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("list business unit approvers")
		writeError(w, http.StatusInternalServerError, "failed to load business unit approvers")
		return
	}
	writeJSON(w, http.StatusOK, approvers)
}

func (h *BusinessUnitsHandler) List(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasBusinessUnitAdmin(r.Context()) {
		writeError(w, http.StatusForbidden, "admin or procurement_admin access required")
		return
	}
	units, err := h.Repo.ListBusinessUnits(r.Context())
	if err != nil {
		reqLog(r).Error().Err(err).Msg("list business units")
		writeError(w, http.StatusInternalServerError, "failed to list business units")
		return
	}
	writeJSON(w, http.StatusOK, units)
}

func (h *BusinessUnitsHandler) Create(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasBusinessUnitAdmin(r.Context()) {
		writeError(w, http.StatusForbidden, "admin or procurement_admin access required")
		return
	}
	var in businessUnitInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	repoIn, errMsg := in.toRepo()
	if errMsg != "" {
		writeError(w, http.StatusBadRequest, errMsg)
		return
	}
	user := middleware.UserFromCtx(r.Context())
	c, err := h.Repo.CreateBusinessUnit(r.Context(), repoIn, user.ID)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("create business unit")
		writeError(w, http.StatusInternalServerError, "failed to create business unit")
		return
	}
	recordAuditEvent(r, h.Repo, model.AuditCreateBusinessUnit, "", model.EntityBusinessUnit, &c.ID, c.Name)
	writeJSON(w, http.StatusCreated, c)
}

func (h *BusinessUnitsHandler) Get(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasBusinessUnitAdmin(r.Context()) {
		writeError(w, http.StatusForbidden, "admin or procurement_admin access required")
		return
	}
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid business unit id")
		return
	}
	c, err := h.Repo.GetBusinessUnit(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "business unit not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load business unit")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *BusinessUnitsHandler) Update(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasBusinessUnitAdmin(r.Context()) {
		writeError(w, http.StatusForbidden, "admin or procurement_admin access required")
		return
	}
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid business unit id")
		return
	}
	var in businessUnitInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	repoIn, errMsg := in.toRepo()
	if errMsg != "" {
		writeError(w, http.StatusBadRequest, errMsg)
		return
	}
	if err := h.Repo.UpdateBusinessUnit(r.Context(), id, repoIn); err != nil {
		reqLog(r).Error().Err(err).Msg("update business unit")
		writeError(w, http.StatusInternalServerError, "failed to update business unit")
		return
	}
	recordAuditEvent(r, h.Repo, model.AuditUpdateBusinessUnit, "", model.EntityBusinessUnit, &id, repoIn.Name)
	c, err := h.Repo.GetBusinessUnit(r.Context(), id)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("reload business unit after mutation")
		writeError(w, http.StatusInternalServerError, "failed to reload business unit")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// Usage returns a "where used" summary (count of referencing purchase requests)
// for a business unit. Management page only.
func (h *BusinessUnitsHandler) Usage(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasBusinessUnitAdmin(r.Context()) {
		writeError(w, http.StatusForbidden, "admin or procurement_admin access required")
		return
	}
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid business unit id")
		return
	}
	usage, err := h.Repo.GetBusinessUnitUsage(r.Context(), id)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("business unit usage")
		writeError(w, http.StatusInternalServerError, "failed to load business unit usage")
		return
	}
	writeJSON(w, http.StatusOK, usage)
}

// Invoices returns the invoices allocated to a business unit, bucketed by status
// (pending/approved/paid) with the business unit's allocated share totalled per
// currency. Management page only.
func (h *BusinessUnitsHandler) Invoices(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasBusinessUnitAdmin(r.Context()) {
		writeError(w, http.StatusForbidden, "admin or procurement_admin access required")
		return
	}
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid business unit id")
		return
	}
	summary, err := h.Repo.GetBusinessUnitInvoiceSummary(r.Context(), id)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("business unit invoices")
		writeError(w, http.StatusInternalServerError, "failed to load business unit invoices")
		return
	}
	writeJSON(w, http.StatusOK, summary)
}
