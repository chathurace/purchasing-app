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

type ConfigOptionsHandler struct {
	Repo *repository.Repository
	Log  zerolog.Logger
}

type configOptionInput struct {
	ListKey   string `json:"list_key"`
	Value     string `json:"value"`
	SortOrder int    `json:"sort_order"`
	IsActive  bool   `json:"is_active"`
}

// configLookupResponse is the shape consumed by the requisition-form dropdowns
// and the Settings page: the active values grouped by list, plus the registry of
// known lists (key + label) so the frontend isn't hardcoded.
type configLookupResponse struct {
	Lists map[string][]string `json:"lists"`
	Keys  []model.ConfigList  `json:"keys"`
}

// Lookup returns the active option values grouped by list_key. Any authenticated
// user may call it (used to populate dropdowns).
func (h *ConfigOptionsHandler) Lookup(w http.ResponseWriter, r *http.Request) {
	lists, err := h.Repo.ConfigOptionsLookup(r.Context())
	if err != nil {
		reqLog(r).Error().Err(err).Msg("config options lookup")
		writeError(w, http.StatusInternalServerError, "failed to load options")
		return
	}
	writeJSON(w, http.StatusOK, configLookupResponse{Lists: lists, Keys: model.ConfigLists})
}

// List returns every option (active and inactive) for the Settings page.
func (h *ConfigOptionsHandler) List(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasCostCenterAdmin(r.Context()) {
		writeError(w, http.StatusForbidden, "admin or finance_admin access required")
		return
	}
	opts, err := h.Repo.ListConfigOptions(r.Context())
	if err != nil {
		reqLog(r).Error().Err(err).Msg("list config options")
		writeError(w, http.StatusInternalServerError, "failed to list options")
		return
	}
	writeJSON(w, http.StatusOK, opts)
}

func (h *ConfigOptionsHandler) Create(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasCostCenterAdmin(r.Context()) {
		writeError(w, http.StatusForbidden, "admin or finance_admin access required")
		return
	}
	var in configOptionInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	key := strings.TrimSpace(in.ListKey)
	value := strings.TrimSpace(in.Value)
	if !model.IsConfigListKey(key) {
		writeError(w, http.StatusBadRequest, "unknown list")
		return
	}
	if value == "" {
		writeError(w, http.StatusBadRequest, "value is required")
		return
	}
	o, err := h.Repo.CreateConfigOption(r.Context(), repository.ConfigOptionInput{
		ListKey: key, Value: value, SortOrder: in.SortOrder, IsActive: in.IsActive,
	})
	if err != nil {
		if errors.Is(err, repository.ErrDuplicateValue) {
			writeError(w, http.StatusConflict, "this value already exists in the list")
			return
		}
		reqLog(r).Error().Err(err).Msg("create config option")
		writeError(w, http.StatusInternalServerError, "failed to create option")
		return
	}
	writeJSON(w, http.StatusCreated, o)
}

func (h *ConfigOptionsHandler) Update(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasCostCenterAdmin(r.Context()) {
		writeError(w, http.StatusForbidden, "admin or finance_admin access required")
		return
	}
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid option id")
		return
	}
	var in configOptionInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	value := strings.TrimSpace(in.Value)
	if value == "" {
		writeError(w, http.StatusBadRequest, "value is required")
		return
	}
	// list_key is immutable; only value/sort_order/is_active are editable.
	if err := h.Repo.UpdateConfigOption(r.Context(), id, repository.ConfigOptionInput{
		Value: value, SortOrder: in.SortOrder, IsActive: in.IsActive,
	}); err != nil {
		if errors.Is(err, repository.ErrDuplicateValue) {
			writeError(w, http.StatusConflict, "this value already exists in the list")
			return
		}
		reqLog(r).Error().Err(err).Msg("update config option")
		writeError(w, http.StatusInternalServerError, "failed to update option")
		return
	}
	o, err := h.Repo.GetConfigOption(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "option not found")
			return
		}
		reqLog(r).Error().Err(err).Msg("reload option after mutation")
		writeError(w, http.StatusInternalServerError, "failed to reload option")
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func (h *ConfigOptionsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasCostCenterAdmin(r.Context()) {
		writeError(w, http.StatusForbidden, "admin or finance_admin access required")
		return
	}
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid option id")
		return
	}
	if err := h.Repo.DeleteConfigOption(r.Context(), id); err != nil {
		reqLog(r).Error().Err(err).Msg("delete config option")
		writeError(w, http.StatusInternalServerError, "failed to delete option")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
