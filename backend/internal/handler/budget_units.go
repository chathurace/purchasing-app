package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/cs/purchasing-app/internal/middleware"
	"github.com/cs/purchasing-app/internal/model"
	"github.com/cs/purchasing-app/internal/repository"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"
)

type BudgetUnitsHandler struct {
	Repo *repository.Repository
	Log  zerolog.Logger
}

type bracketInput struct {
	Currency    string   `json:"currency"`
	MinValue    float64  `json:"min_value"`
	MaxValue    *float64 `json:"max_value"` // null = unbounded
	ApproverIDs []int64  `json:"approver_ids"`
}

type budgetUnitInput struct {
	Code              string         `json:"code"`
	Name              string         `json:"name"`
	Description       string         `json:"description"`
	Budget            float64        `json:"budget"`
	Currency          string         `json:"currency"`
	IsActive          bool           `json:"is_active"`
	DefaultApproverID *int64         `json:"default_approver_id"`
	Brackets          []bracketInput `json:"brackets"`
}

// toRepo validates and builds the repository input. It returns the input and a
// user-facing error message (empty on success).
func (in budgetUnitInput) toRepo() (repository.BudgetUnitInput, string) {
	if strings.TrimSpace(in.Name) == "" {
		return repository.BudgetUnitInput{}, "budget unit name is required"
	}
	if in.DefaultApproverID == nil {
		return repository.BudgetUnitInput{}, "a default approver is required"
	}
	brackets := make([]repository.BudgetUnitBracketInput, 0, len(in.Brackets))
	for _, b := range in.Brackets {
		if strings.TrimSpace(b.Currency) == "" {
			return repository.BudgetUnitInput{}, "each bracket needs a currency"
		}
		if len(b.ApproverIDs) == 0 {
			return repository.BudgetUnitInput{}, "each bracket needs at least one approver"
		}
		if b.MaxValue != nil && *b.MaxValue < b.MinValue {
			return repository.BudgetUnitInput{}, "a bracket's max value cannot be below its min value"
		}
		// Dedupe approver ids, preserving order.
		seen := map[int64]bool{}
		ids := make([]int64, 0, len(b.ApproverIDs))
		for _, id := range b.ApproverIDs {
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
		brackets = append(brackets, repository.BudgetUnitBracketInput{
			Currency:    strings.TrimSpace(b.Currency),
			MinValue:    b.MinValue,
			MaxValue:    b.MaxValue,
			ApproverIDs: ids,
		})
	}
	return repository.BudgetUnitInput{
		Code:              strings.TrimSpace(in.Code),
		Name:              strings.TrimSpace(in.Name),
		Description:       in.Description,
		Budget:            in.Budget,
		Currency:          strings.TrimSpace(in.Currency),
		IsActive:          in.IsActive,
		DefaultApproverID: in.DefaultApproverID,
		Brackets:          brackets,
	}, ""
}

// Lookup returns active budget units as summaries for the purchase-request
// dropdown. Any authenticated user may call it.
func (h *BudgetUnitsHandler) Lookup(w http.ResponseWriter, r *http.Request) {
	units, err := h.Repo.ListActiveBudgetUnits(r.Context())
	if err != nil {
		reqLog(r).Error().Err(err).Msg("lookup budget units")
		writeError(w, http.StatusInternalServerError, "failed to list budget units")
		return
	}
	writeJSON(w, http.StatusOK, units)
}

// Approvers resolves the qualified budget approver(s) for a budget unit at a
// given estimated value + currency — the creation-phase preview shown to the
// requester. `value` is optional (blank → the highest bracket is assumed).
// Any authenticated user may call it.
func (h *BudgetUnitsHandler) Approvers(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid budget unit id")
		return
	}
	var value *float64
	if raw := strings.TrimSpace(r.URL.Query().Get("value")); raw != "" {
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "value must be a number")
			return
		}
		value = &v
	}
	currency := strings.TrimSpace(r.URL.Query().Get("currency"))
	approvers, err := h.Repo.BudgetApproversForValue(r.Context(), id, value, currency)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("resolve budget approvers")
		writeError(w, http.StatusInternalServerError, "failed to resolve budget approvers")
		return
	}
	writeJSON(w, http.StatusOK, approvers)
}

func (h *BudgetUnitsHandler) List(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasBudgetUnitAdmin(r.Context()) {
		writeError(w, http.StatusForbidden, "admin or procurement_admin access required")
		return
	}
	units, err := h.Repo.ListBudgetUnits(r.Context())
	if err != nil {
		reqLog(r).Error().Err(err).Msg("list budget units")
		writeError(w, http.StatusInternalServerError, "failed to list budget units")
		return
	}
	writeJSON(w, http.StatusOK, units)
}

func (h *BudgetUnitsHandler) Create(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasBudgetUnitAdmin(r.Context()) {
		writeError(w, http.StatusForbidden, "admin or procurement_admin access required")
		return
	}
	var in budgetUnitInput
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
	c, err := h.Repo.CreateBudgetUnit(r.Context(), repoIn, user.ID)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("create budget unit")
		writeError(w, http.StatusInternalServerError, "failed to create budget unit")
		return
	}
	recordAuditEvent(r, h.Repo, model.AuditCreateBudgetUnit, "", model.EntityBudgetUnit, &c.ID, c.Name)
	writeJSON(w, http.StatusCreated, c)
}

func (h *BudgetUnitsHandler) Get(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasBudgetUnitAdmin(r.Context()) {
		writeError(w, http.StatusForbidden, "admin or procurement_admin access required")
		return
	}
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid budget unit id")
		return
	}
	c, err := h.Repo.GetBudgetUnit(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "budget unit not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load budget unit")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *BudgetUnitsHandler) Update(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasBudgetUnitAdmin(r.Context()) {
		writeError(w, http.StatusForbidden, "admin or procurement_admin access required")
		return
	}
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid budget unit id")
		return
	}
	var in budgetUnitInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	repoIn, errMsg := in.toRepo()
	if errMsg != "" {
		writeError(w, http.StatusBadRequest, errMsg)
		return
	}
	if err := h.Repo.UpdateBudgetUnit(r.Context(), id, repoIn); err != nil {
		reqLog(r).Error().Err(err).Msg("update budget unit")
		writeError(w, http.StatusInternalServerError, "failed to update budget unit")
		return
	}
	recordAuditEvent(r, h.Repo, model.AuditUpdateBudgetUnit, "", model.EntityBudgetUnit, &id, repoIn.Name)
	c, err := h.Repo.GetBudgetUnit(r.Context(), id)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("reload budget unit after mutation")
		writeError(w, http.StatusInternalServerError, "failed to reload budget unit")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// Usage returns a "where used" summary (count of referencing purchase requests)
// for a budget unit. Management page only.
func (h *BudgetUnitsHandler) Usage(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasBudgetUnitAdmin(r.Context()) {
		writeError(w, http.StatusForbidden, "admin or procurement_admin access required")
		return
	}
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid budget unit id")
		return
	}
	usage, err := h.Repo.GetBudgetUnitUsage(r.Context(), id)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("budget unit usage")
		writeError(w, http.StatusInternalServerError, "failed to load budget unit usage")
		return
	}
	writeJSON(w, http.StatusOK, usage)
}

// Invoices returns the invoices allocated to a budget unit, bucketed by status
// (pending/approved/paid) with the budget unit's allocated share totalled per
// currency. Management page only.
func (h *BudgetUnitsHandler) Invoices(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasBudgetUnitAdmin(r.Context()) {
		writeError(w, http.StatusForbidden, "admin or procurement_admin access required")
		return
	}
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid budget unit id")
		return
	}
	summary, err := h.Repo.GetBudgetUnitInvoiceSummary(r.Context(), id)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("budget unit invoices")
		writeError(w, http.StatusInternalServerError, "failed to load budget unit invoices")
		return
	}
	writeJSON(w, http.StatusOK, summary)
}
