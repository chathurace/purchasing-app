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

// TeamsHandler serves the Teams settings section: the Legal/Security/Procurement
// groups, their shared email, and their membership (which is the team's member
// role — adding/removing a member grants/revokes that role).
type TeamsHandler struct {
	Repo *repository.Repository
	Log  zerolog.Logger
}

// requireTeamAdmin writes a 403 and returns false unless the caller may manage
// teams (procurement_admin or admin).
func (h *TeamsHandler) requireTeamAdmin(w http.ResponseWriter, r *http.Request) bool {
	if !middleware.HasTeamAdmin(r.Context()) {
		writeError(w, http.StatusForbidden, "procurement_admin or admin access required")
		return false
	}
	return true
}

// List returns every team with its members. Open to any authenticated user — the
// assignee dropdown on the approval cards needs to read team membership.
func (h *TeamsHandler) List(w http.ResponseWriter, r *http.Request) {
	teams, err := h.Repo.ListTeams(r.Context())
	if err != nil {
		reqLog(r).Error().Err(err).Msg("list teams")
		writeError(w, http.StatusInternalServerError, "failed to list teams")
		return
	}
	if teams == nil {
		teams = []*repository.Team{}
	}
	writeJSON(w, http.StatusOK, teams)
}

type teamEmailInput struct {
	TeamEmail string `json:"team_email"`
}

// UpdateEmail sets a team's shared email address (procurement_admin/admin).
func (h *TeamsHandler) UpdateEmail(w http.ResponseWriter, r *http.Request) {
	if !h.requireTeamAdmin(w, r) {
		return
	}
	key := urlParam(r, "key")
	var in teamEmailInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.TeamEmail))
	if email != "" && !looksLikeEmail(email) {
		writeError(w, http.StatusBadRequest, "a valid email address is required")
		return
	}
	if err := h.Repo.SetTeamEmail(r.Context(), key, email); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "team not found")
			return
		}
		reqLog(r).Error().Err(err).Str("team", key).Msg("set team email")
		writeError(w, http.StatusInternalServerError, "failed to update team")
		return
	}
	h.writeTeam(w, r, key)
}

type teamMemberInput struct {
	UserID int64 `json:"user_id"`
}

// AddMember adds a user to a team by granting the team's member role
// (procurement_admin/admin).
func (h *TeamsHandler) AddMember(w http.ResponseWriter, r *http.Request) {
	if !h.requireTeamAdmin(w, r) {
		return
	}
	key := urlParam(r, "key")
	team, err := h.Repo.GetTeamByKey(r.Context(), key)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "team not found")
			return
		}
		reqLog(r).Error().Err(err).Str("team", key).Msg("add member: load team")
		writeError(w, http.StatusInternalServerError, "failed to add member")
		return
	}
	var in teamMemberInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if _, err := h.Repo.GetUserByID(r.Context(), in.UserID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		reqLog(r).Error().Err(err).Int64("target_user_id", in.UserID).Msg("add member: look up user")
		writeError(w, http.StatusInternalServerError, "failed to add member")
		return
	}
	if err := h.Repo.EnsureUserHasRole(r.Context(), in.UserID, team.MemberRole); err != nil {
		reqLog(r).Error().Err(err).Msg("add team member")
		writeError(w, http.StatusInternalServerError, "failed to add member")
		return
	}
	recordAuditEvent(r, h.Repo, model.AuditGrantRole, team.MemberRole, model.EntityUser, &in.UserID, "team:"+key)
	h.writeTeam(w, r, key)
}

// RemoveMember removes a user from a team by revoking the team's member role
// (procurement_admin/admin).
func (h *TeamsHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	if !h.requireTeamAdmin(w, r) {
		return
	}
	key := urlParam(r, "key")
	team, err := h.Repo.GetTeamByKey(r.Context(), key)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "team not found")
			return
		}
		reqLog(r).Error().Err(err).Str("team", key).Msg("remove member: load team")
		writeError(w, http.StatusInternalServerError, "failed to remove member")
		return
	}
	userID, err := parseID(r, "userID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	if err := h.Repo.RemoveUserRole(r.Context(), userID, team.MemberRole); err != nil {
		reqLog(r).Error().Err(err).Msg("remove team member")
		writeError(w, http.StatusInternalServerError, "failed to remove member")
		return
	}
	recordAuditEvent(r, h.Repo, model.AuditRevokeRole, team.MemberRole, model.EntityUser, &userID, "team:"+key)
	h.writeTeam(w, r, key)
}

// writeTeam reloads and returns a single team after a mutation.
func (h *TeamsHandler) writeTeam(w http.ResponseWriter, r *http.Request, key string) {
	team, err := h.Repo.GetTeamByKey(r.Context(), key)
	if err != nil {
		reqLog(r).Error().Err(err).Str("team", key).Msg("reload team")
		writeError(w, http.StatusInternalServerError, "failed to reload team")
		return
	}
	writeJSON(w, http.StatusOK, team)
}
