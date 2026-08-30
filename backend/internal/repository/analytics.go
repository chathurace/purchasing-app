package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// ── BPM analytics reads (docs/bpm-analytics.md) ──────────────────────────────
//
// Analytics is a cross-organisation window for admin / procurement_admin — the
// same audience (and the same underlying process_events rows) as the audit log.
// So unlike ListPurchaseRequests these reads deliberately apply NO visibility
// gate: no team-lead gate, no caller scope, no per-caller binds at all. They are
// read-only and never touch the event write path.

// AnalyticsPR is one purchase request as the analytics views see it: identity,
// when it started, and the two people who own it. Deliberately narrow — the
// process timeline, not the request's contents, is what these pages are about.
// It doubles as the header of the per-PR flow page.
type AnalyticsPR struct {
	ID        int64        `json:"id"`
	Reference *string      `json:"reference"`
	Title     string       `json:"title"`
	Status    string       `json:"status"`
	Priority  string       `json:"priority"`
	CreatedAt time.Time    `json:"created_at"`
	Requester *UserSummary `json:"requester"`
	// Assignee is nil while the PR is unassigned (before procurement claims it).
	Assignee *UserSummary `json:"assignee"`
}

// AnalyticsPRSort names a sortable column of the analytics PR list. Anything
// else falls back to the created-at default.
type AnalyticsPRSort string

const (
	AnalyticsPRSortCreated   AnalyticsPRSort = "created_at"
	AnalyticsPRSortRequester AnalyticsPRSort = "requester"
	AnalyticsPRSortAssignee  AnalyticsPRSort = "assignee"
)

const (
	defaultAnalyticsPRLimit = 500
	maxAnalyticsPRLimit     = 2000
)

// analyticsPRLimit clamps a requested page size into [1, maxAnalyticsPRLimit],
// so a missing or absurd ?limit= can neither return nothing nor scan the table.
func analyticsPRLimit(limit int) int {
	switch {
	case limit <= 0:
		return defaultAnalyticsPRLimit
	case limit > maxAnalyticsPRLimit:
		return maxAnalyticsPRLimit
	default:
		return limit
	}
}

// analyticsPROrderBy maps a sort column + direction to SQL. The column is
// resolved through this switch (never interpolated from the request) and the
// direction is one of two literals, so building the clause by concatenation is
// safe. Ties always break on id so a capped page is deterministic.
func analyticsPROrderBy(sort AnalyticsPRSort, desc bool) string {
	dir := "ASC"
	if desc {
		dir = "DESC"
	}
	switch sort {
	// The person columns sort on the display string the list actually renders —
	// the name, or the email when the name is blank — so the visible order matches
	// the clicked header. NULLS LAST keeps unassigned PRs at the bottom in both
	// directions rather than flooding the first page of an ascending sort.
	case AnalyticsPRSortRequester:
		return fmt.Sprintf(`ORDER BY lower(COALESCE(NULLIF(u.name, ''), u.email)) %s, pr.id DESC`, dir)
	case AnalyticsPRSortAssignee:
		return fmt.Sprintf(`ORDER BY lower(COALESCE(NULLIF(au.name, ''), au.email)) %s NULLS LAST, pr.id DESC`, dir)
	default:
		return fmt.Sprintf(`ORDER BY pr.created_at %s, pr.id %s`, dir, dir)
	}
}

const analyticsPRSelect = `
	SELECT pr.id, pr.reference, pr.title, pr.status, pr.priority, pr.created_at,
	       pr.requester_id, u.email, u.name,
	       pr.assignee_id, au.email, au.name
	FROM purchase_requests pr
	JOIN users u ON u.id = pr.requester_id
	LEFT JOIN users au ON au.id = pr.assignee_id`

// ListAnalyticsPRs returns every purchase request, newest-first by default,
// ordered by the given column and capped at limit rows (see analyticsPRLimit).
// No visibility gate — see the file comment.
func (r *Repository) ListAnalyticsPRs(ctx context.Context, sort AnalyticsPRSort, desc bool, limit int) ([]*AnalyticsPR, error) {
	query := analyticsPRSelect + " " + analyticsPROrderBy(sort, desc) + " LIMIT $1"
	rows, err := r.pool.Query(ctx, query, analyticsPRLimit(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*AnalyticsPR
	for rows.Next() {
		pr, err := scanAnalyticsPR(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, pr)
	}
	return out, rows.Err()
}

// GetAnalyticsPR returns one request's analytics header. Returns pgx.ErrNoRows
// when the id is unknown (the handler maps that to 404).
func (r *Repository) GetAnalyticsPR(ctx context.Context, prID int64) (*AnalyticsPR, error) {
	return scanAnalyticsPR(r.pool.QueryRow(ctx, analyticsPRSelect+` WHERE pr.id = $1`, prID))
}

// scanAnalyticsPR reads one row. The anonymous interface is pgx.Row and pgx.Rows
// both, so the list read and the single-row read share this one scan.
func scanAnalyticsPR(row interface {
	Scan(dest ...any) error
}) (*AnalyticsPR, error) {
	var (
		pr            AnalyticsPR
		reference     pgtype.Text
		requesterID   int64
		email, name   pgtype.Text
		assigneeID    pgtype.Int8
		aEmail, aName pgtype.Text
	)
	if err := row.Scan(&pr.ID, &reference, &pr.Title, &pr.Status, &pr.Priority, &pr.CreatedAt,
		&requesterID, &email, &name, &assigneeID, &aEmail, &aName); err != nil {
		return nil, err
	}
	if reference.Valid {
		pr.Reference = &reference.String
	}
	pr.Requester = &UserSummary{ID: requesterID, Email: email.String, Name: name.String}
	if assigneeID.Valid {
		pr.Assignee = &UserSummary{ID: assigneeID.Int64, Email: aEmail.String, Name: aName.String}
	}
	return &pr, nil
}
