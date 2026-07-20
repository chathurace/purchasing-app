package handler

import (
	"errors"
	"net/http"

	"github.com/cs/purchasing-app/internal/middleware"
	"github.com/cs/purchasing-app/internal/model"
	"github.com/cs/purchasing-app/internal/repository"
)

// --- PR assignment ---
//
// After the team lead has approved a PR, a procurement user must be assigned to it
// before any procurement work (quotations, recommendations) can start. Once assigned,
// ANY procurement/procurement_admin user may work on it — assignment tracks the owner,
// it does not lock others out. A procurement user who does non-readonly work on a PR
// they don't already own is automatically recorded as a collaborator (ensureCollaborator).

// assignmentWorkGate enforces "a PR must be assigned before procurement work can
// start". It returns ok=true when the caller may proceed; otherwise a status code
// and message to write. Call it after the team-lead-approval check and after the
// handler's own HasProcurementAccess check — any procurement user may proceed once
// the PR is assigned.
func assignmentWorkGate(r *http.Request, pr *repository.PurchaseRequest) (int, string, bool) {
	if pr.AssigneeID == nil {
		return http.StatusConflict, "purchase request must be assigned to a procurement user before work can start", false
	}
	return 0, "", true
}

// ensureCollaborator records the caller as a collaborator on the PR when a
// procurement/procurement_admin user does non-readonly work on a PR they don't
// already own (they aren't the assignee or an existing collaborator). Best-effort:
// call it after a successful mutation. Skips the requester and legal/security/budget
// approvers (they aren't procurement) and plain admins (oversight, not ownership).
// Package-level so both PurchaseRequestsHandler and QuotationsHandler can call it.
func ensureCollaborator(r *http.Request, repo *repository.Repository, prID int64) {
	ctx := r.Context()
	// Exact-role check (not middleware.HasRole, which lets admin pass): only
	// procurement / procurement_admin get auto-enrolled.
	isProcurement := false
	for _, role := range middleware.RolesFromCtx(ctx) {
		if role == model.RoleProcurement || role == model.RoleProcurementAdmin {
			isProcurement = true
			break
		}
	}
	if !isProcurement {
		return
	}
	user := middleware.UserFromCtx(ctx)
	if user == nil {
		return
	}
	added, err := repo.EnsurePRCollaborator(ctx, prID, user.ID)
	if err != nil {
		reqLog(r).Error().Err(err).Int64("pr_id", prID).Msg("auto-add collaborator")
		return
	}
	if added {
		recordProcessEvent(r, repo, prID, model.ProcessUpdatePRCollaborators, model.QualifierAssign)
	}
}

// callerCanManageCollaborators reports whether the caller may add/remove
// collaborators: the current assignee, or a procurement_admin/admin.
func (h *PurchaseRequestsHandler) callerCanManageCollaborators(r *http.Request, pr *repository.PurchaseRequest) bool {
	ctx := r.Context()
	if middleware.HasRole(ctx, model.RoleProcurementAdmin) || middleware.HasRole(ctx, model.RoleAdmin) {
		return true
	}
	user := middleware.UserFromCtx(ctx)
	return user != nil && pr.AssigneeID != nil && *pr.AssigneeID == user.ID
}

// attachAssignmentActionable sets the per-caller assignment display flags
// (my_can_assign / my_can_manage_collaborators / my_can_work) on a detail read, so
// the UI shows the assignment controls and gates procurement sections to match the
// backend enforcement. Only procurement users see any of these as true.
func (h *PurchaseRequestsHandler) attachAssignmentActionable(r *http.Request, pr *repository.PurchaseRequest) {
	if pr == nil {
		return
	}
	ctx := r.Context()
	isAdmin := middleware.HasRole(ctx, model.RoleProcurementAdmin) || middleware.HasRole(ctx, model.RoleAdmin)
	user := middleware.UserFromCtx(ctx)
	// Assignment only makes sense once procurement can see the PR.
	if !middleware.HasProcurementAccess(ctx) || !model.IsTeamLeadApproved(pr.TeamLeadStatus) {
		return
	}
	switch {
	case isAdmin:
		pr.MyCanAssign = true
	case user != nil && (pr.AssigneeID == nil || *pr.AssigneeID == user.ID):
		// A plain procurement user can claim an unassigned PR or hand back their own.
		pr.MyCanAssign = true
	}
	pr.MyCanManageCollaborators = h.callerCanManageCollaborators(r, pr)
	// Once assigned, any procurement user may work on the PR (they'll be auto-added
	// as a collaborator when they do). The handler already confirmed procurement access.
	pr.MyCanWork = pr.AssigneeID != nil
}

// SetAssignee assigns (or unassigns) the PR's procurement owner.
//   - assignee_id == null      → unassign; current assignee or procurement_admin/admin.
//   - assignee_id == caller.id → self-assign / claim an unassigned PR; any procurement user.
//   - assignee_id == other     → assign/reassign to another user; procurement_admin/admin only.
//
// Requires team-lead approval first (procurement can't see the PR before then).
func (h *PurchaseRequestsHandler) SetAssignee(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasProcurementAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "procurement access required")
		return
	}
	pr, ok := h.load(w, r)
	if !ok {
		return
	}
	if !model.IsTeamLeadApproved(pr.TeamLeadStatus) {
		writeError(w, http.StatusConflict, "purchase request is awaiting team lead approval")
		return
	}
	var in struct {
		AssigneeID *int64 `json:"assignee_id"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ctx := r.Context()
	user := middleware.UserFromCtx(ctx)
	isAdmin := middleware.HasRole(ctx, model.RoleProcurementAdmin) || middleware.HasRole(ctx, model.RoleAdmin)
	switch {
	case in.AssigneeID == nil:
		// Unassign: current assignee or admin.
		if !isAdmin && !(pr.AssigneeID != nil && user != nil && *pr.AssigneeID == user.ID) {
			writeError(w, http.StatusForbidden, "only the current assignee or a procurement_admin can unassign")
			return
		}
	case isAdmin:
		// May assign to anyone.
	case user != nil && *in.AssigneeID == user.ID && (pr.AssigneeID == nil || *pr.AssigneeID == user.ID):
		// Plain procurement user claiming an unassigned PR (or re-affirming their own).
	default:
		writeError(w, http.StatusForbidden, "only a procurement_admin can assign to another user or reassign")
		return
	}
	if err := h.Repo.SetPRAssignee(ctx, pr.ID, in.AssigneeID, user.ID); err != nil {
		if errors.Is(err, repository.ErrNotProcurementUser) {
			writeError(w, http.StatusBadRequest, "the assignee must be a member of the procurement team")
			return
		}
		reqLog(r).Error().Err(err).Msg("set PR assignee")
		writeError(w, http.StatusInternalServerError, "failed to update assignee")
		return
	}
	qualifier := model.QualifierAssign
	if in.AssigneeID == nil {
		qualifier = model.QualifierUnassign
	}
	recordProcessEvent(r, h.Repo, pr.ID, model.ProcessAssignPR, qualifier)
	if in.AssigneeID != nil {
		h.notifyPRAssignment(r, pr, *in.AssigneeID, "assignee")
	}
	h.reloadPR(w, r, pr.ID)
}

// AddCollaborator adds a procurement user as a collaborator (full work access).
// Managed by the current assignee or a procurement_admin/admin.
func (h *PurchaseRequestsHandler) AddCollaborator(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.load(w, r)
	if !ok {
		return
	}
	if !h.callerCanManageCollaborators(r, pr) {
		writeError(w, http.StatusForbidden, "only the assignee or a procurement_admin can manage collaborators")
		return
	}
	var in struct {
		UserID int64 `json:"user_id"`
	}
	if err := decodeJSON(r, &in); err != nil || in.UserID == 0 {
		writeError(w, http.StatusBadRequest, "a user is required")
		return
	}
	user := middleware.UserFromCtx(r.Context())
	if err := h.Repo.AddPRCollaborator(r.Context(), pr.ID, in.UserID, user.ID); err != nil {
		if errors.Is(err, repository.ErrNotProcurementUser) {
			writeError(w, http.StatusBadRequest, "a collaborator must be a member of the procurement team")
			return
		}
		reqLog(r).Error().Err(err).Msg("add PR collaborator")
		writeError(w, http.StatusInternalServerError, "failed to add collaborator")
		return
	}
	recordProcessEvent(r, h.Repo, pr.ID, model.ProcessUpdatePRCollaborators, model.QualifierAssign)
	h.notifyPRAssignment(r, pr, in.UserID, "collaborator")
	h.reloadPR(w, r, pr.ID)
}

// RemoveCollaborator removes a collaborator from the PR. Managed by the current
// assignee or a procurement_admin/admin.
func (h *PurchaseRequestsHandler) RemoveCollaborator(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.load(w, r)
	if !ok {
		return
	}
	if !h.callerCanManageCollaborators(r, pr) {
		writeError(w, http.StatusForbidden, "only the assignee or a procurement_admin can manage collaborators")
		return
	}
	userID, err := parseID(r, "userID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	if err := h.Repo.RemovePRCollaborator(r.Context(), pr.ID, userID); err != nil {
		reqLog(r).Error().Err(err).Msg("remove PR collaborator")
		writeError(w, http.StatusInternalServerError, "failed to remove collaborator")
		return
	}
	recordProcessEvent(r, h.Repo, pr.ID, model.ProcessUpdatePRCollaborators, model.QualifierUnassign)
	h.reloadPR(w, r, pr.ID)
}
