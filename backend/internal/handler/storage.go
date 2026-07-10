package handler

import (
	"net/http"
	"strings"

	"github.com/cs/purchasing-app/internal/crypto"
	"github.com/cs/purchasing-app/internal/middleware"
	"github.com/cs/purchasing-app/internal/model"
	"github.com/cs/purchasing-app/internal/repository"
	"github.com/cs/purchasing-app/internal/storage"
	"github.com/rs/zerolog"
)

// StorageHandler exposes admin-only endpoints to configure Google Drive file
// storage from the app (Settings → File storage): connect a Google account, pick
// a base folder, and view live status. See docs/file-storage.md.
type StorageHandler struct {
	Repo     *repository.Repository
	Manager  *storage.StorageManager
	Secrets  *crypto.Secretbox // nil when security.secret_key is not configured
	ClientID string
	APIKey   string
	AppID    string
	Log      zerolog.Logger
}

func (h *StorageHandler) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if !middleware.HasRole(r.Context(), model.RoleAdmin) {
		writeError(w, http.StatusForbidden, "admin access required")
		return false
	}
	return true
}

// statusResponse is the live status plus deployment capability flags.
func (h *StorageHandler) statusResponse() map[string]any {
	st := h.Manager.Status()
	return map[string]any{
		"status":                  st,
		"oauth_client_configured": h.Manager.HasOAuthClient(),
		"secret_key_configured":   h.Secrets != nil,
	}
}

// Status returns the current storage configuration and health.
func (h *StorageHandler) Status(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	writeJSON(w, http.StatusOK, h.statusResponse())
}

// Params returns the non-secret bits the browser needs to run the Google Identity
// Services code client and the Picker.
func (h *StorageHandler) Params(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	settings, err := h.Repo.GetStorageSettings(r.Context())
	if err != nil {
		reqLog(r).Error().Err(err).Msg("get storage settings")
		writeError(w, http.StatusInternalServerError, "failed to read settings")
		return
	}
	appID := h.AppID
	if appID == "" {
		// Default to the GCP project number — the client id is "<projnum>-xxxx".
		if i := strings.IndexByte(h.ClientID, '-'); i > 0 {
			appID = h.ClientID[:i]
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"client_id":     h.ClientID,
		"api_key":       h.APIKey,
		"app_id":        appID,
		"connected":     settings.Exists && settings.RefreshTokenEnc != "",
		"account_email": settings.GoogleAccountEmail,
	})
}

// Connect exchanges a browser OAuth authorization code for a refresh token,
// encrypts it, and stores it. It does not require a folder yet; if one is already
// configured, the live store is reconfigured with the new token.
func (h *StorageHandler) Connect(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	if h.Secrets == nil {
		writeError(w, http.StatusBadRequest, "security.secret_key must be set in config.yaml to store credentials")
		return
	}
	var in struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(r, &in); err != nil || strings.TrimSpace(in.Code) == "" {
		writeError(w, http.StatusBadRequest, "code is required")
		return
	}

	ctx := r.Context()
	refreshToken, err := h.Manager.ExchangeCode(ctx, in.Code, "postmessage")
	if err != nil {
		reqLog(r).Warn().Err(err).Msg("storage connect: exchange code")
		writeError(w, http.StatusBadRequest, "failed to connect account: "+err.Error())
		return
	}
	email, _ := h.Manager.ProbeAccount(ctx, refreshToken) // best-effort

	enc, err := h.Secrets.Encrypt(refreshToken)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("encrypt refresh token")
		writeError(w, http.StatusInternalServerError, "failed to store credentials")
		return
	}

	settings, err := h.Repo.GetStorageSettings(ctx)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("read storage settings")
		writeError(w, http.StatusInternalServerError, "failed to read settings")
		return
	}
	settings.Backend = "gdrive"
	settings.RefreshTokenEnc = enc
	settings.GoogleAccountEmail = email
	user := middleware.UserFromCtx(ctx)
	if err := h.Repo.UpsertStorageSettings(ctx, *settings, user.ID); err != nil {
		reqLog(r).Error().Err(err).Msg("save storage settings")
		writeError(w, http.StatusInternalServerError, "failed to save settings")
		return
	}

	// If a folder was already chosen, swap the live store to the new account.
	if settings.BaseFolderID != "" {
		if err := h.Manager.ReconfigureGDriveOAuth(ctx, refreshToken, settings.BaseFolderID, email); err != nil {
			reqLog(r).Warn().Err(err).Msg("reconfigure after connect")
		}
	}
	writeJSON(w, http.StatusOK, h.statusResponse())
}

// SetFolder sets the base folder (granted via the Picker), validates access with
// the connected account's token, and hot-swaps the live store.
func (h *StorageHandler) SetFolder(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	if h.Secrets == nil {
		writeError(w, http.StatusBadRequest, "security.secret_key must be set in config.yaml")
		return
	}
	var in struct {
		BaseFolderID string `json:"base_folder_id"`
		Name         string `json:"name"`
	}
	if err := decodeJSON(r, &in); err != nil || strings.TrimSpace(in.BaseFolderID) == "" {
		writeError(w, http.StatusBadRequest, "base_folder_id is required")
		return
	}

	ctx := r.Context()
	settings, err := h.Repo.GetStorageSettings(ctx)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("read storage settings")
		writeError(w, http.StatusInternalServerError, "failed to read settings")
		return
	}
	if !settings.Exists || settings.RefreshTokenEnc == "" {
		writeError(w, http.StatusBadRequest, "connect a Google account first")
		return
	}
	refreshToken, err := h.Secrets.Decrypt(settings.RefreshTokenEnc)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("decrypt refresh token")
		writeError(w, http.StatusInternalServerError, "stored credentials could not be read (was security.secret_key changed?)")
		return
	}

	if err := h.Manager.ReconfigureGDriveOAuth(ctx, refreshToken, in.BaseFolderID, settings.GoogleAccountEmail); err != nil {
		writeError(w, http.StatusBadRequest, "could not use that folder: "+err.Error())
		return
	}

	st := h.Manager.Status()
	settings.Backend = "gdrive"
	settings.BaseFolderID = st.BaseFolderID
	settings.BaseFolderName = st.BaseFolderName
	if st.AccountEmail != "" {
		settings.GoogleAccountEmail = st.AccountEmail
	}
	user := middleware.UserFromCtx(ctx)
	if err := h.Repo.UpsertStorageSettings(ctx, *settings, user.ID); err != nil {
		reqLog(r).Error().Err(err).Msg("save storage settings")
		writeError(w, http.StatusInternalServerError, "storage configured but failed to persist settings")
		return
	}
	writeJSON(w, http.StatusOK, h.statusResponse())
}
