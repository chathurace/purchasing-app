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

type UsersHandler struct {
	Repo *repository.Repository
	Log  zerolog.Logger
}

// Me returns the signed-in user's profile, roles, and the is_approver capability
// flag that drives approver-only nav tabs (Approvals/Quotations/Contracts). The
// flag is true when the caller is an approver on any PR they didn't submit — a
// signal the role list alone can't provide, since budget owners and named
// approvers hold no distinguishing role.
func (h *UsersHandler) Me(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := middleware.UserFromCtx(ctx)
	roles := middleware.RolesFromCtx(ctx)
	if roles == nil {
		roles = []string{}
	}
	isApprover, err := h.Repo.HasApprovableWork(ctx, user.ID,
		middleware.HasRole(ctx, model.RoleLegal), middleware.HasRole(ctx, model.RoleSecurity))
	if err != nil {
		reqLog(r).Error().Err(err).Msg("resolve is_approver")
		writeError(w, http.StatusInternalServerError, "failed to load profile")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":          user.ID,
		"sub":         user.Sub,
		"email":       user.Email,
		"name":        user.Name,
		"roles":       roles,
		"is_approver": isApprover,
	})
}

// Lookup returns a lightweight directory of active users (id/email/name) for the
// approver picker. Available to any authenticated user — it exposes no roles or
// lifecycle flags, only what's needed to choose an approver.
func (h *UsersHandler) Lookup(w http.ResponseWriter, r *http.Request) {
	users, err := h.Repo.ListActiveUsers(r.Context())
	if err != nil {
		reqLog(r).Error().Err(err).Msg("lookup users")
		writeError(w, http.StatusInternalServerError, "failed to list users")
		return
	}
	writeJSON(w, http.StatusOK, users)
}

// requireAdmin writes a 403 and returns false when the caller is not an admin.
func (h *UsersHandler) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if !middleware.HasRole(r.Context(), model.RoleAdmin) {
		writeError(w, http.StatusForbidden, "admin access required")
		return false
	}
	return true
}

// List returns all users with their roles (admin only).
func (h *UsersHandler) List(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	users, err := h.Repo.ListUsersWithRoles(r.Context())
	if err != nil {
		reqLog(r).Error().Err(err).Msg("list users")
		writeError(w, http.StatusInternalServerError, "failed to list users")
		return
	}
	if users == nil {
		users = []*repository.AdminUser{}
	}
	writeJSON(w, http.StatusOK, users)
}

type createUserInput struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

// Create pre-creates a user from an email address (admin only). The account is
// linked to its OIDC identity on that person's first login.
func (h *UsersHandler) Create(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	var in createUserInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if !looksLikeEmail(email) {
		writeError(w, http.StatusBadRequest, "a valid email address is required")
		return
	}
	u, err := h.Repo.CreateInvitedUser(r.Context(), email, strings.TrimSpace(in.Name))
	if err != nil {
		if errors.Is(err, repository.ErrEmailExists) {
			writeError(w, http.StatusConflict, "a user with this email already exists")
			return
		}
		reqLog(r).Error().Err(err).Msg("create invited user")
		writeError(w, http.StatusInternalServerError, "failed to create user")
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

type roleInput struct {
	Role string `json:"role"`
}

// AddRole grants an assignable role to a user (admin only).
func (h *UsersHandler) AddRole(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	userID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	var in roleInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !model.IsAssignableRole(in.Role) {
		writeError(w, http.StatusBadRequest, "unknown or non-assignable role")
		return
	}
	if _, err := h.Repo.GetUserByID(r.Context(), userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		reqLog(r).Error().Err(err).Int64("target_user_id", userID).Msg("add role: look up user")
		writeError(w, http.StatusInternalServerError, "failed to add role")
		return
	}
	if err := h.Repo.EnsureUserHasRole(r.Context(), userID, in.Role); err != nil {
		reqLog(r).Error().Err(err).Msg("add user role")
		writeError(w, http.StatusInternalServerError, "failed to add role")
		return
	}
	h.writeUser(w, r, userID)
}

// RemoveRole revokes a role from a user (admin only). `staff` is a permanent
// baseline and the last admin cannot be demoted (lockout guard).
func (h *UsersHandler) RemoveRole(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	userID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	role := urlParam(r, "role")
	if role == model.RoleStaff {
		writeError(w, http.StatusBadRequest, "the staff role is a baseline and cannot be removed")
		return
	}
	if !model.IsAssignableRole(role) {
		writeError(w, http.StatusBadRequest, "unknown or non-assignable role")
		return
	}
	if role == model.RoleAdmin {
		ok, err := h.Repo.OtherActiveAdminExists(r.Context(), userID)
		if err != nil {
			reqLog(r).Error().Err(err).Msg("check other admins")
			writeError(w, http.StatusInternalServerError, "failed to remove role")
			return
		}
		if !ok {
			writeError(w, http.StatusConflict, "cannot remove the last admin")
			return
		}
	}
	if err := h.Repo.RemoveUserRole(r.Context(), userID, role); err != nil {
		reqLog(r).Error().Err(err).Msg("remove user role")
		writeError(w, http.StatusInternalServerError, "failed to remove role")
		return
	}
	h.writeUser(w, r, userID)
}

type activeInput struct {
	Active bool `json:"active"`
}

// SetActive activates or deactivates a user (admin only). An admin cannot
// deactivate themselves, nor the last remaining active admin.
func (h *UsersHandler) SetActive(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	userID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	var in activeInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !in.Active {
		if self := middleware.UserFromCtx(r.Context()); self != nil && self.ID == userID {
			writeError(w, http.StatusBadRequest, "you cannot deactivate your own account")
			return
		}
		ok, err := h.Repo.OtherActiveAdminExists(r.Context(), userID)
		if err != nil {
			reqLog(r).Error().Err(err).Msg("check other admins")
			writeError(w, http.StatusInternalServerError, "failed to update user")
			return
		}
		// Only blocks when the *target* is itself an admin and no other active
		// admin remains; OtherActiveAdminExists excludes the target either way.
		if !ok && userIsAdmin(r, h, userID) {
			writeError(w, http.StatusConflict, "cannot deactivate the last admin")
			return
		}
	}
	if err := h.Repo.SetUserActive(r.Context(), userID, in.Active); err != nil {
		reqLog(r).Error().Err(err).Msg("set user active")
		writeError(w, http.StatusInternalServerError, "failed to update user")
		return
	}
	h.writeUser(w, r, userID)
}

// writeUser reloads and returns the user's admin view after a mutation.
func (h *UsersHandler) writeUser(w http.ResponseWriter, r *http.Request, userID int64) {
	users, err := h.Repo.ListUsersWithRoles(r.Context())
	if err != nil {
		reqLog(r).Error().Err(err).Msg("reload users")
		writeError(w, http.StatusInternalServerError, "failed to reload user")
		return
	}
	for _, u := range users {
		if u.ID == userID {
			writeJSON(w, http.StatusOK, u)
			return
		}
	}
	writeError(w, http.StatusNotFound, "user not found")
}

// userIsAdmin reports whether the target user currently holds the admin role.
func userIsAdmin(r *http.Request, h *UsersHandler, userID int64) bool {
	roles, err := h.Repo.GetUserRoles(r.Context(), userID)
	if err != nil {
		return false
	}
	for _, role := range roles {
		if role == model.RoleAdmin {
			return true
		}
	}
	return false
}

// looksLikeEmail does a minimal sanity check (exactly one @, non-empty parts).
func looksLikeEmail(s string) bool {
	at := strings.IndexByte(s, '@')
	return at > 0 && at < len(s)-1 && strings.IndexByte(s[at+1:], '@') == -1 && !strings.ContainsAny(s, " ")
}
