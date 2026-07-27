package handler

import (
	"net/http"

	"github.com/rs/zerolog"

	"github.com/cs/purchasing-app/internal/middleware"
	"github.com/cs/purchasing-app/internal/model"
	"github.com/cs/purchasing-app/internal/repository"
)

// HomeHandler backs the role-based home page (GET /api/v1/home): a read-only
// summary of count tiles + a recent-activity feed, scoped to the caller's roles.
type HomeHandler struct {
	Repo *repository.Repository
	Log  zerolog.Logger
}

// homeActivityLimit caps each "latest activity" feed.
const homeActivityLimit = 8

// StaffHome is the "my requests" block, shown to every user (anyone can submit).
type StaffHome struct {
	MyRequestsCount int                       `json:"my_requests_count"`
	CompletedCount  int                       `json:"completed_count"`
	RecentActivity  []repository.HomeActivity `json:"recent_activity"`
}

// ApprovalsHome is the approver block, shown when the caller approves any PR they
// did not submit (named approver or budget/legal/security card actor).
type ApprovalsHome struct {
	PendingCount   int                       `json:"pending_count"`
	CompletedCount int                       `json:"completed_count"`
	RecentActivity []repository.HomeActivity `json:"recent_activity"`
}

// ProcurementHome is the procurement block, shown to procurement/admin users.
type ProcurementHome struct {
	PendingCount          int                       `json:"pending_count"`
	AwaitingDeliveryCount int                       `json:"awaiting_delivery_count"`
	CompletedCount        int                       `json:"completed_count"`
	RecentActivity        []repository.HomeActivity `json:"recent_activity"`
}

// HomeResponse bundles whichever role blocks apply to the caller.
type HomeResponse struct {
	Staff       *StaffHome       `json:"staff,omitempty"`
	Approvals   *ApprovalsHome   `json:"approvals,omitempty"`
	Procurement *ProcurementHome `json:"procurement,omitempty"`
}

// Get assembles the caller's home dashboard. The staff block is always present;
// the approvals and procurement blocks are added when the caller qualifies.
func (h *HomeHandler) Get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := middleware.UserFromCtx(ctx)
	hasLegal := middleware.HasRole(ctx, model.RoleLegal)
	hasSecurity := middleware.HasRole(ctx, model.RoleSecurity)
	isAdmin := middleware.HasRole(ctx, model.RoleAdmin)

	resp := HomeResponse{}

	// Staff — everyone.
	total, completed, err := h.Repo.CountMyRequests(ctx, user.ID)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("home: count my requests")
		writeError(w, http.StatusInternalServerError, "failed to load home")
		return
	}
	activity, err := h.Repo.RecentActivityForRequester(ctx, user.ID, homeActivityLimit)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("home: requester activity")
		writeError(w, http.StatusInternalServerError, "failed to load home")
		return
	}
	resp.Staff = &StaffHome{
		MyRequestsCount: total,
		CompletedCount:  completed,
		RecentActivity:  coalesceActivity(activity),
	}

	// Approvals — anyone who approves a PR they did not submit.
	isApprover, err := h.Repo.HasApprovableWork(ctx, user.ID, user.Email, hasLegal, hasSecurity)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("home: is approver")
		writeError(w, http.StatusInternalServerError, "failed to load home")
		return
	}
	if isApprover {
		pending, done, err := h.Repo.CountApprovals(ctx, user.ID, user.Email, hasLegal, hasSecurity)
		if err != nil {
			reqLog(r).Error().Err(err).Msg("home: count approvals")
			writeError(w, http.StatusInternalServerError, "failed to load home")
			return
		}
		act, err := h.Repo.RecentActivityForApprover(ctx, user.ID, user.Email, hasLegal, hasSecurity, homeActivityLimit)
		if err != nil {
			reqLog(r).Error().Err(err).Msg("home: approver activity")
			writeError(w, http.StatusInternalServerError, "failed to load home")
			return
		}
		resp.Approvals = &ApprovalsHome{
			PendingCount:   pending,
			CompletedCount: done,
			RecentActivity: coalesceActivity(act),
		}
	}

	// Procurement — procurement/procurement_admin/admin.
	if middleware.HasProcurementAccess(ctx) {
		pending, awaiting, done, err := h.Repo.CountProcurementRequests(ctx, isAdmin)
		if err != nil {
			reqLog(r).Error().Err(err).Msg("home: count procurement requests")
			writeError(w, http.StatusInternalServerError, "failed to load home")
			return
		}
		act, err := h.Repo.RecentActivityForProcurement(ctx, isAdmin, homeActivityLimit)
		if err != nil {
			reqLog(r).Error().Err(err).Msg("home: procurement activity")
			writeError(w, http.StatusInternalServerError, "failed to load home")
			return
		}
		resp.Procurement = &ProcurementHome{
			PendingCount:          pending,
			AwaitingDeliveryCount: awaiting,
			CompletedCount:        done,
			RecentActivity:        coalesceActivity(act),
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// coalesceActivity never emits a null JSON array for an empty feed.
func coalesceActivity(a []repository.HomeActivity) []repository.HomeActivity {
	if a == nil {
		return []repository.HomeActivity{}
	}
	return a
}
