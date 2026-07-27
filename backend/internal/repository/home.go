package repository

import (
	"context"
	"strconv"
	"time"
)

// Home dashboard reads — counts and recent-activity feeds backing the role-based
// home page (GET /api/v1/home). Everything here is read-only aggregation that
// reuses the same SQL predicate fragments as ListPurchaseRequests
// (approvablePredicate, myApprovalStateExpr, teamLeadMatch) with the identical
// bind order: $1 = callerID, $2 = hasLegal, $3 = hasSecurity, $4 = lowercased
// caller email.

// HomeActivity is one recent process event, joined to its purchase request so the
// UI can render "<ref> — <action> by <actor>" and link to the PR.
type HomeActivity struct {
	PurchaseRequestID int64     `json:"purchase_request_id"`
	Reference         string    `json:"reference"`
	Title             string    `json:"title"`
	Action            string    `json:"action"`
	Qualifier         string    `json:"qualifier"`
	ActorEmail        string    `json:"actor_email"`
	CreatedAt         time.Time `json:"created_at"`
}

// CountMyRequests returns the caller's total submitted requests and how many are
// "completed" — order_signed (the terminal procurement state) or the reserved
// completed status.
func (r *Repository) CountMyRequests(ctx context.Context, callerID int64) (total, completed int, err error) {
	err = r.pool.QueryRow(ctx, `
		SELECT count(*),
		       count(*) FILTER (WHERE status IN ('order_signed', 'completed'))
		FROM purchase_requests
		WHERE requester_id = $1`, callerID).Scan(&total, &completed)
	return total, completed, err
}

// CountApprovals buckets the PRs awaiting or decided by the caller as an approver
// (any of the three approval systems) into pending vs completed (approved or
// rejected). The PR set matches PRScopeApprovals in ListPurchaseRequests.
func (r *Repository) CountApprovals(ctx context.Context, callerID int64, callerEmail string, hasLegal, hasSecurity bool) (pending, completed int, err error) {
	err = r.pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE state = 'pending'),
		       count(*) FILTER (WHERE state <> 'pending')
		FROM (
			SELECT (`+myApprovalStateExpr+`) AS state
			FROM purchase_requests pr
			WHERE pr.requester_id <> $1 AND (`+teamLeadMatch+`
				OR (pr.team_lead_status = 'approved' AND `+approvablePredicate+`))
		) t`,
		callerID, hasLegal, hasSecurity, normEmail(callerEmail)).Scan(&pending, &completed)
	return pending, completed, err
}

// procurementVisibleWhere is the PR-set predicate for the procurement work queue:
// admins see every PR, other procurement users see every team-lead-approved PR.
// Takes no bind parameters (the admin flag is resolved in Go).
func procurementVisibleWhere(isAdmin bool) string {
	if isAdmin {
		return `TRUE`
	}
	return `pr.team_lead_status = 'approved'`
}

// CountProcurementRequests buckets the procurement work queue (rejected/cancelled
// excluded) into: pending (active, before the order is signed), awaiting delivery
// (order signed but not yet fully paid), and completed (order signed with at least
// one invoice and every invoice paid).
func (r *Repository) CountProcurementRequests(ctx context.Context, isAdmin bool) (pending, awaiting, completed int, err error) {
	err = r.pool.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE status IN ('submitted', 'under_review', 'vendor_selected', 'contract_prepared')),
			count(*) FILTER (WHERE status IN ('order_signed', 'completed') AND NOT fully_paid),
			count(*) FILTER (WHERE status IN ('order_signed', 'completed') AND fully_paid)
		FROM (
			SELECT pr.status,
			       (EXISTS (SELECT 1 FROM invoices i WHERE i.purchase_request_id = pr.id)
			        AND NOT EXISTS (SELECT 1 FROM invoices i WHERE i.purchase_request_id = pr.id AND i.status <> 'paid')) AS fully_paid
			FROM purchase_requests pr
			WHERE pr.status NOT IN ('rejected', 'cancelled') AND `+procurementVisibleWhere(isAdmin)+`
		) t`).Scan(&pending, &awaiting, &completed)
	return pending, awaiting, completed, err
}

// recentActivity returns the most recent process events over the PRs matched by
// `where` (a boolean fragment referencing $1..$N as supplied in args). The LIMIT
// is appended as the next positional parameter. Newest first.
func (r *Repository) recentActivity(ctx context.Context, where string, args []any, limit int) ([]HomeActivity, error) {
	q := `
		SELECT pe.purchase_request_id, COALESCE(pr.reference, ''), pr.title,
		       pe.action, pe.qualifier, pe.actor_email, pe.created_at
		FROM process_events pe
		JOIN purchase_requests pr ON pr.id = pe.purchase_request_id
		WHERE ` + where + `
		ORDER BY pe.created_at DESC, pe.id DESC
		LIMIT $` + strconv.Itoa(len(args)+1)
	rows, err := r.pool.Query(ctx, q, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HomeActivity
	for rows.Next() {
		var a HomeActivity
		if err := rows.Scan(&a.PurchaseRequestID, &a.Reference, &a.Title,
			&a.Action, &a.Qualifier, &a.ActorEmail, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// RecentActivityForRequester returns recent events on the caller's own requests.
func (r *Repository) RecentActivityForRequester(ctx context.Context, callerID int64, limit int) ([]HomeActivity, error) {
	return r.recentActivity(ctx, `pr.requester_id = $1`, []any{callerID}, limit)
}

// RecentActivityForApprover returns recent events on the PRs awaiting or decided
// by the caller as an approver (same set as CountApprovals).
func (r *Repository) RecentActivityForApprover(ctx context.Context, callerID int64, callerEmail string, hasLegal, hasSecurity bool, limit int) ([]HomeActivity, error) {
	where := `pr.requester_id <> $1 AND (` + teamLeadMatch +
		` OR (pr.team_lead_status = 'approved' AND ` + approvablePredicate + `))`
	return r.recentActivity(ctx, where, []any{callerID, hasLegal, hasSecurity, normEmail(callerEmail)}, limit)
}

// RecentActivityForProcurement returns recent events on the procurement work queue
// (same visibility as CountProcurementRequests).
func (r *Repository) RecentActivityForProcurement(ctx context.Context, isAdmin bool, limit int) ([]HomeActivity, error) {
	return r.recentActivity(ctx, procurementVisibleWhere(isAdmin), nil, limit)
}
