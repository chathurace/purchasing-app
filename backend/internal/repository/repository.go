package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cs/purchasing-app/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Pool() *pgxpool.Pool { return r.pool }

// --- Users & roles ---

type User struct {
	ID        int64     `json:"id"`
	Sub       string    `json:"sub"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (r *Repository) UpsertUser(ctx context.Context, sub, email, name string) (*User, error) {
	// Pass NULL instead of "" so IdPs that don't emit email/name don't collide
	// on the empty string. Emails are stored lowercased so the case-insensitive
	// unique index and the login email-match resolve to one row.
	emailArg, nameArg := emailArgOf(email), nilIfEmpty(name)
	row := r.pool.QueryRow(ctx, `
		INSERT INTO users (sub, email, name)
		VALUES ($1, $2, $3)
		ON CONFLICT (sub) DO UPDATE
		SET email = COALESCE(EXCLUDED.email, users.email),
		    name  = COALESCE(EXCLUDED.name, users.name),
		    updated_at = NOW()
		RETURNING id, sub, email, name, is_active, created_at, updated_at`, sub, emailArg, nameArg)
	return scanUser(row)
}

func (r *Repository) GetUserByID(ctx context.Context, id int64) (*User, error) {
	row := r.pool.QueryRow(ctx, `SELECT id, sub, email, name, is_active, created_at, updated_at FROM users WHERE id = $1`, id)
	return scanUser(row)
}

func (r *Repository) GetUserRoles(ctx context.Context, userID int64) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT r.name FROM roles r
		JOIN user_roles ur ON ur.role_id = r.id
		WHERE ur.user_id = $1
		ORDER BY r.name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var roles []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		roles = append(roles, name)
	}
	return roles, rows.Err()
}

// EnsureUserHasRole grants the named role to the user if not already present.
// Idempotent — safe to call on every login.
func (r *Repository) EnsureUserHasRole(ctx context.Context, userID int64, roleName string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO user_roles (user_id, role_id)
		SELECT $1, id FROM roles WHERE name = $2
		ON CONFLICT DO NOTHING`, userID, roleName)
	return err
}

// GrantDefaultRoleIfNone grants roleName to the user only when they currently
// have no roles at all. This auto-provisions first-time users without ever
// overriding an admin's later role changes (e.g. a demoted user keeps their set).
func (r *Repository) GrantDefaultRoleIfNone(ctx context.Context, userID int64, roleName string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO user_roles (user_id, role_id)
		SELECT $1, id FROM roles WHERE name = $2
		  AND NOT EXISTS (SELECT 1 FROM user_roles WHERE user_id = $1)
		ON CONFLICT DO NOTHING`, userID, roleName)
	return err
}

// ProvisionUserOnLogin resolves the OIDC identity to a user row, in one tx:
//
//  1. Known sub      → refresh email/name, return the row.
//  2. Pending invite → an admin-created row (sub IS NULL) whose email matches:
//     claim it by filling in the real sub. This is how "add user by email"
//     links to the eventual login.
//  3. Otherwise       → insert a brand-new self-provisioned user.
//
// It does NOT enforce is_active — the caller gates login on the returned flag so
// a deactivated user is still resolved (and stays linked) but refused entry.
func (r *Repository) ProvisionUserOnLogin(ctx context.Context, sub, email, name string) (*User, error) {
	emailArg, nameArg := emailArgOf(email), nilIfEmpty(name)

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var id int64
	err = tx.QueryRow(ctx, `SELECT id FROM users WHERE sub = $1`, sub).Scan(&id)
	switch {
	case err == nil:
		// (1) Existing user — refresh profile fields if the IdP supplied them.
		if _, err = tx.Exec(ctx, `
			UPDATE users
			SET email = COALESCE($2, email), name = COALESCE($3, name), updated_at = NOW()
			WHERE id = $1`, id, emailArg, nameArg); err != nil {
			return nil, err
		}
	case errors.Is(err, pgx.ErrNoRows):
		// (2) Try to claim a pending invite by email.
		claimed := false
		if emailArg != nil {
			err = tx.QueryRow(ctx, `
				UPDATE users
				SET sub = $1, name = COALESCE($3, name), updated_at = NOW()
				WHERE sub IS NULL AND lower(email) = $2
				RETURNING id`, sub, emailArg, nameArg).Scan(&id)
			switch {
			case err == nil:
				claimed = true
			case errors.Is(err, pgx.ErrNoRows):
				// fall through to insert
			default:
				return nil, err
			}
		}
		// (3) Brand-new user.
		if !claimed {
			if err = tx.QueryRow(ctx, `
				INSERT INTO users (sub, email, name) VALUES ($1, $2, $3)
				RETURNING id`, sub, emailArg, nameArg).Scan(&id); err != nil {
				return nil, err
			}
		}
	default:
		return nil, err
	}

	row := tx.QueryRow(ctx, `
		SELECT id, sub, email, name, is_active, created_at, updated_at FROM users WHERE id = $1`, id)
	u, err := scanUser(row)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return u, nil
}

// --- Admin user management ---

// AdminUser is a user row enriched with roles and lifecycle flags for the admin
// management view.
type AdminUser struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	IsActive  bool      `json:"is_active"`
	Pending   bool      `json:"pending"` // never logged in (no OIDC sub yet)
	Roles     []string  `json:"roles"`
	CreatedAt time.Time `json:"created_at"`
}

// ListUsersWithRoles returns every user with their roles, for the admin page.
func (r *Repository) ListUsersWithRoles(ctx context.Context) ([]*AdminUser, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT u.id, u.email, u.name, u.is_active, (u.sub IS NULL) AS pending, u.created_at,
		       COALESCE(array_agg(rl.name ORDER BY rl.name) FILTER (WHERE rl.name IS NOT NULL), '{}') AS roles
		FROM users u
		LEFT JOIN user_roles ur ON ur.user_id = u.id
		LEFT JOIN roles rl ON rl.id = ur.role_id
		GROUP BY u.id
		ORDER BY lower(u.email), u.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*AdminUser
	for rows.Next() {
		au := &AdminUser{}
		var email, name pgtype.Text
		if err := rows.Scan(&au.ID, &email, &name, &au.IsActive, &au.Pending, &au.CreatedAt, &au.Roles); err != nil {
			return nil, err
		}
		au.Email = email.String
		au.Name = name.String
		out = append(out, au)
	}
	return out, rows.Err()
}

// ErrEmailExists is returned when an admin invites an email already on file.
var ErrEmailExists = errors.New("a user with this email already exists")

// CreateInvitedUser pre-creates a user from an email (no OIDC sub yet). The row
// is claimed on that person's first login (see ProvisionUserOnLogin).
func (r *Repository) CreateInvitedUser(ctx context.Context, email, name string) (*User, error) {
	emailArg, nameArg := emailArgOf(email), nilIfEmpty(name)
	row := r.pool.QueryRow(ctx, `
		INSERT INTO users (sub, email, name) VALUES (NULL, $1, $2)
		RETURNING id, sub, email, name, is_active, created_at, updated_at`, emailArg, nameArg)
	u, err := scanUser(row)
	if err != nil {
		var pgErr interface{ SQLState() string }
		if errors.As(err, &pgErr) && pgErr.SQLState() == "23505" {
			return nil, ErrEmailExists
		}
		return nil, err
	}
	return u, nil
}

// RemoveUserRole revokes a role from a user (idempotent).
func (r *Repository) RemoveUserRole(ctx context.Context, userID int64, roleName string) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM user_roles
		WHERE user_id = $1 AND role_id = (SELECT id FROM roles WHERE name = $2)`, userID, roleName)
	return err
}

// SetUserActive toggles a user's active flag (false blocks login).
func (r *Repository) SetUserActive(ctx context.Context, userID int64, active bool) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE users SET is_active = $2, updated_at = NOW() WHERE id = $1`, userID, active)
	return err
}

// OtherActiveAdminExists reports whether an active admin other than excludeID
// exists. Used to block removing/deactivating the last admin (lockout guard).
func (r *Repository) OtherActiveAdminExists(ctx context.Context, excludeID int64) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM user_roles ur
			JOIN roles rl ON rl.id = ur.role_id
			JOIN users u  ON u.id = ur.user_id
			WHERE rl.name = $1 AND u.is_active AND u.id <> $2
		)`, model.RoleAdmin, excludeID).Scan(&exists)
	return exists, err
}

func scanUser(row pgx.Row) (*User, error) {
	u := &User{}
	var sub, email, name pgtype.Text
	if err := row.Scan(&u.ID, &sub, &email, &name, &u.IsActive, &u.CreatedAt, &u.UpdatedAt); err != nil {
		return nil, err
	}
	u.Sub = sub.String   // empty for invited users who have never logged in
	u.Email = email.String
	u.Name = name.String
	return u, nil
}

// emailArgOf returns a lowercased, trimmed email for binding, or nil (SQL NULL)
// when empty — so IdPs that omit the email claim don't collide on "".
func emailArgOf(email string) any {
	e := strings.ToLower(strings.TrimSpace(email))
	if e == "" {
		return nil
	}
	return e
}

// nilIfEmpty returns the trimmed string, or nil (SQL NULL) when empty.
func nilIfEmpty(s string) any {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return s
}

// --- Purchase requests ---

type Item struct {
	ID          int64   `json:"id"`
	Description string  `json:"description"`
	Quantity    float64 `json:"quantity"`
	Position    int     `json:"position"`
}

type Link struct {
	ID       int64  `json:"id"`
	URL      string `json:"url"`
	Label    string `json:"label"`
	Position int    `json:"position"`
}

type Document struct {
	ID          int64     `json:"id"`
	Filename    string    `json:"filename"`
	StoredPath  string    `json:"-"`
	ContentType string    `json:"content_type"`
	SizeBytes   int64     `json:"size_bytes"`
	// Notes is an optional free-text note attached to the document (used by the
	// contract card for each draft/signed PDF).
	Notes      string    `json:"notes"`
	UploadedBy *int64    `json:"uploaded_by,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type PurchaseRequest struct {
	ID              int64      `json:"id"`
	// Reference is the human-readable PR-YYYY-NNNNNNN identifier assigned at
	// submission. Nil for requests created before the feature (legacy display).
	Reference       *string    `json:"reference"`
	Title           string     `json:"title"`
	RequesterID     int64      `json:"requester_id"`
	CostCenter      string     `json:"cost_center"`
	CostCenterID    *int64     `json:"cost_center_id"`
	Comments        string     `json:"comments"`
	Status          string     `json:"status"`
	RejectionReason string     `json:"rejection_reason"`
	// Requisition form fields (WSO2 SOP-85000). Core/queryable fields are columns;
	// the remaining structured form data lives in Details (opaque JSON).
	Team                string          `json:"team"`
	Entity              string          `json:"entity"`
	Category            string          `json:"category"` // IT | NON-IT
	EstimatedValue      float64         `json:"estimated_value"`
	Currency            string          `json:"currency"`
	BudgetApproverName  string          `json:"budget_approver_name"`
	BudgetApproverEmail string          `json:"budget_approver_email"`
	Details             json.RawMessage `json:"details"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	Items           []Item     `json:"items"`
	Links           []Link     `json:"links"`
	Documents       []Document `json:"documents"`
	// Requester is populated on detail/list reads for display.
	Requester *UserSummary `json:"requester,omitempty"`
	// Approvals is the full per-approver decision list (detail reads only).
	Approvals []PRApproval `json:"approvals,omitempty"`
	// Approval tally for list/summary views.
	ApprovalsTotal    int `json:"approvals_total"`
	ApprovalsApproved int `json:"approvals_approved"`
	// MyApprovalStatus is the calling user's own decision on this PR (pending |
	// approved | rejected), or nil when the caller is not an approver. Populated
	// on list reads so a user can spot requests awaiting their approval.
	MyApprovalStatus *string `json:"my_approval_status,omitempty"`
	// MyApprovalState is the caller's unified state across BOTH approval systems —
	// the named pr_approvals rows and the budget/legal/security recommendation
	// cards — one of pending | approved | rejected. Drives the Approvals-tab
	// filter. Populated on list reads; only meaningful for PRs the caller is
	// actually an approver on (i.e. the scope=approvals list).
	MyApprovalState *string `json:"my_approval_state,omitempty"`
	// Recommendation is the PR's procurement recommendation (detail reads only),
	// or nil when none has been added.
	Recommendation *Recommendation `json:"recommendation,omitempty"`
}

type UserSummary struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

// PurchaseRequestInput carries the writable fields for create/update.
type PurchaseRequestInput struct {
	Title      string
	CostCenter string
	// CostCenterID links the request to a managed cost center. When set, the
	// cost center's name is mirrored into the free-text CostCenter column for
	// display continuity; when nil, the free-text CostCenter is used as-is.
	CostCenterID *int64
	Comments     string
	Items      []Item
	Links      []Link
	// ApproverIDs are the users asked to approve the request. Honored only on
	// create — approvers are managed afterwards through dedicated endpoints so an
	// edit never discards recorded decisions.
	ApproverIDs []int64
	// Requisition form fields. Details is the opaque JSON blob for everything not
	// promoted to a column (validated/shaped on the client).
	Team                string
	Entity              string
	Category            string
	EstimatedValue      float64
	Currency            string
	BudgetApproverName  string
	BudgetApproverEmail string
	Details             json.RawMessage
}

// detailsOrEmpty returns the details JSON, defaulting to an empty object so the
// NOT NULL jsonb column always gets valid JSON.
func detailsOrEmpty(d json.RawMessage) string {
	if len(d) == 0 {
		return "{}"
	}
	return string(d)
}

// CreatePurchaseRequest inserts the request plus its items and links in one tx.
func (r *Repository) CreatePurchaseRequest(ctx context.Context, requesterID int64, in PurchaseRequestInput) (*PurchaseRequest, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Assign the human-readable reference from a per-year sequence. The
	// ON CONFLICT DO UPDATE bumps the counter atomically under a row lock so
	// concurrent submissions in the same year never reuse a number.
	year := time.Now().Year()
	var seq int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO pr_reference_sequences (year, last_seq)
		VALUES ($1, 1)
		ON CONFLICT (year) DO UPDATE SET last_seq = pr_reference_sequences.last_seq + 1
		RETURNING last_seq`, year).Scan(&seq); err != nil {
		return nil, err
	}
	reference := fmt.Sprintf("PR-%04d-%07d", year, seq)

	var id int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO purchase_requests
			(title, requester_id, cost_center, cost_center_id, comments,
			 team, entity, category, estimated_value, currency,
			 budget_approver_name, budget_approver_email, details, reference)
		VALUES ($1, $2, COALESCE((SELECT name FROM cost_centers WHERE id = $4), $3), $4, $5,
			 $6, $7, $8, $9, $10, $11, $12, $13::jsonb, $14)
		RETURNING id`,
		in.Title, requesterID, in.CostCenter, in.CostCenterID, in.Comments,
		in.Team, in.Entity, in.Category, in.EstimatedValue, in.Currency,
		in.BudgetApproverName, in.BudgetApproverEmail, detailsOrEmpty(in.Details), reference).Scan(&id); err != nil {
		return nil, err
	}
	if err := replaceItemsLinks(ctx, tx, id, in); err != nil {
		return nil, err
	}
	for _, approverID := range in.ApproverIDs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO pr_approvals (purchase_request_id, approver_id)
			VALUES ($1, $2) ON CONFLICT DO NOTHING`, id, approverID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return r.GetPurchaseRequest(ctx, id)
}

// UpdatePurchaseRequest updates the writable fields and replaces items/links.
func (r *Repository) UpdatePurchaseRequest(ctx context.Context, id int64, in PurchaseRequestInput) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE purchase_requests
		SET title = $2,
			cost_center = COALESCE((SELECT name FROM cost_centers WHERE id = $4), $3),
			cost_center_id = $4, comments = $5,
			team = $6, entity = $7, category = $8, estimated_value = $9, currency = $10,
			budget_approver_name = $11, budget_approver_email = $12, details = $13::jsonb,
			updated_at = NOW()
		WHERE id = $1`,
		id, in.Title, in.CostCenter, in.CostCenterID, in.Comments,
		in.Team, in.Entity, in.Category, in.EstimatedValue, in.Currency,
		in.BudgetApproverName, in.BudgetApproverEmail, detailsOrEmpty(in.Details)); err != nil {
		return err
	}
	if err := replaceItemsLinks(ctx, tx, id, in); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// replaceItemsLinks deletes and re-inserts the items and links for a request.
func replaceItemsLinks(ctx context.Context, tx pgx.Tx, prID int64, in PurchaseRequestInput) error {
	if _, err := tx.Exec(ctx, `DELETE FROM pr_items WHERE purchase_request_id = $1`, prID); err != nil {
		return err
	}
	for i, it := range in.Items {
		if _, err := tx.Exec(ctx, `
			INSERT INTO pr_items (purchase_request_id, description, quantity, position)
			VALUES ($1, $2, $3, $4)`, prID, it.Description, it.Quantity, i); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM pr_links WHERE purchase_request_id = $1`, prID); err != nil {
		return err
	}
	for i, ln := range in.Links {
		if _, err := tx.Exec(ctx, `
			INSERT INTO pr_links (purchase_request_id, url, label, position)
			VALUES ($1, $2, $3, $4)`, prID, ln.URL, ln.Label, i); err != nil {
			return err
		}
	}
	return nil
}

// PRListScope selects how ListPurchaseRequests filters rows for the caller.
type PRListScope string

const (
	// PRScopeDefault preserves the legacy behavior: finance/admin (seesAll) get
	// every PR, everyone else gets the union of their own submissions plus any
	// PR awaiting their approval. Used by callers that don't pass ?scope=.
	PRScopeDefault PRListScope = ""
	// PRScopeMine returns only PRs the caller submitted (any role) — the
	// "Requests" tab for staff/approvers.
	PRScopeMine PRListScope = "mine"
	// PRScopeApprovals returns only PRs awaiting the caller's decision (they are
	// an approver but not the requester) — the "Approvals" tab.
	PRScopeApprovals PRListScope = "approvals"
)

// approvablePredicate is a SQL boolean fragment, true when the caller is an
// approver on the purchase request aliased `pr`: a named pr_approvals approver, a
// legal/security recommendation-card actor, or the budget owner of the PR's cost
// center. It binds $1 (caller user id), $2 (hasLegal), $3 (hasSecurity); callers
// MUST supply those three args in that order.
const approvablePredicate = `(
	EXISTS (SELECT 1 FROM pr_approvals a WHERE a.purchase_request_id = pr.id AND a.approver_id = $1)
	OR EXISTS (
		SELECT 1 FROM pr_recommendation_approvals ra
		JOIN pr_recommendations rec ON rec.id = ra.recommendation_id
		WHERE rec.purchase_request_id = pr.id AND (
			(ra.approval_type = 'legal'    AND $2)
			OR (ra.approval_type = 'security' AND $3)
			OR (ra.approval_type = 'budget' AND pr.cost_center_id IS NOT NULL AND EXISTS (
				SELECT 1 FROM cost_centers cc
				WHERE cc.id = pr.cost_center_id AND (
					cc.primary_owner_id = $1
					OR EXISTS (SELECT 1 FROM cost_center_secondary_owners s
					           WHERE s.cost_center_id = cc.id AND s.user_id = $1))))
		)))`

// myApprovalStateExpr is a SQL scalar expression resolving the caller's unified
// approval state across both systems: 'pending' if any named row or actionable
// card is still outstanding, else 'rejected' if the caller rejected a named row
// (cards cannot be rejected), else 'approved'. Binds $1/$2/$3 as above. Only
// meaningful for PRs the caller actually approves; harmless ('approved') otherwise.
const myApprovalStateExpr = `
	CASE
		WHEN EXISTS (SELECT 1 FROM pr_approvals a WHERE a.purchase_request_id = pr.id AND a.approver_id = $1 AND a.status = 'pending')
		  OR EXISTS (
			SELECT 1 FROM pr_recommendation_approvals ra
			JOIN pr_recommendations rec ON rec.id = ra.recommendation_id
			WHERE rec.purchase_request_id = pr.id AND ra.approved_by IS NULL AND (
				(ra.approval_type = 'legal'    AND $2)
				OR (ra.approval_type = 'security' AND $3)
				OR (ra.approval_type = 'budget' AND pr.cost_center_id IS NOT NULL AND EXISTS (
					SELECT 1 FROM cost_centers cc
					WHERE cc.id = pr.cost_center_id AND (
						cc.primary_owner_id = $1
						OR EXISTS (SELECT 1 FROM cost_center_secondary_owners s
						           WHERE s.cost_center_id = cc.id AND s.user_id = $1))))))
		THEN 'pending'
		WHEN EXISTS (SELECT 1 FROM pr_approvals a WHERE a.purchase_request_id = pr.id AND a.approver_id = $1 AND a.status = 'rejected')
		THEN 'rejected'
		ELSE 'approved'
	END`

// ListPurchaseRequests returns request summaries (no items/links/documents) with
// an approval tally, the caller's own named-approval status, and their unified
// approval state on each. callerID/hasLegal/hasSecurity resolve those per-caller
// fields; scope selects which rows are returned (see PRListScope). seesAll only
// applies to PRScopeDefault.
func (r *Repository) ListPurchaseRequests(ctx context.Context, callerID int64, seesAll, hasLegal, hasSecurity bool, scope PRListScope) ([]*PurchaseRequest, error) {
	query := `
		SELECT pr.id, pr.reference, pr.title, pr.requester_id, pr.cost_center, pr.cost_center_id, pr.comments, pr.status,
		       pr.rejection_reason, pr.created_at, pr.updated_at, u.email, u.name,
		       pr.team, pr.entity, pr.category, pr.estimated_value, pr.currency,
		       (SELECT count(*) FROM pr_approvals a WHERE a.purchase_request_id = pr.id) AS approvals_total,
		       (SELECT count(*) FROM pr_approvals a WHERE a.purchase_request_id = pr.id AND a.status = 'approved') AS approvals_approved,
		       (SELECT a.status FROM pr_approvals a WHERE a.purchase_request_id = pr.id AND a.approver_id = $1) AS my_approval_status,
		       (` + myApprovalStateExpr + `) AS my_approval_state
		FROM purchase_requests pr
		JOIN users u ON u.id = pr.requester_id`
	// $1/$2/$3 are always bound because myApprovalStateExpr references all three.
	args := []any{callerID, hasLegal, hasSecurity}
	switch scope {
	case PRScopeMine:
		query += ` WHERE pr.requester_id = $1`
	case PRScopeApprovals:
		query += ` WHERE pr.requester_id <> $1 AND ` + approvablePredicate
	default:
		if !seesAll {
			query += ` WHERE (pr.requester_id = $1 OR ` + approvablePredicate + `)`
		}
	}
	query += ` ORDER BY pr.created_at DESC`

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*PurchaseRequest
	for rows.Next() {
		pr := &PurchaseRequest{}
		var email, name, myStatus, myState, reference pgtype.Text
		if err := rows.Scan(&pr.ID, &reference, &pr.Title, &pr.RequesterID, &pr.CostCenter, &pr.CostCenterID, &pr.Comments,
			&pr.Status, &pr.RejectionReason, &pr.CreatedAt, &pr.UpdatedAt, &email, &name,
			&pr.Team, &pr.Entity, &pr.Category, &pr.EstimatedValue, &pr.Currency,
			&pr.ApprovalsTotal, &pr.ApprovalsApproved, &myStatus, &myState); err != nil {
			return nil, err
		}
		if reference.Valid {
			pr.Reference = &reference.String
		}
		pr.Requester = &UserSummary{ID: pr.RequesterID, Email: email.String, Name: name.String}
		if myStatus.Valid {
			s := myStatus.String
			pr.MyApprovalStatus = &s
		}
		if myState.Valid {
			s := myState.String
			pr.MyApprovalState = &s
		}
		out = append(out, pr)
	}
	return out, rows.Err()
}

// IsApproverForPR reports whether the caller is an approver on a single PR — the
// per-PR form of approvablePredicate. Used to gate read access to a PR's
// quotations and contracts.
func (r *Repository) IsApproverForPR(ctx context.Context, prID, callerID int64, hasLegal, hasSecurity bool) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM purchase_requests pr
			WHERE pr.id = $4 AND `+approvablePredicate+`)`,
		callerID, hasLegal, hasSecurity, prID).Scan(&ok)
	return ok, err
}

// HasApprovableWork reports whether the caller is an approver on any PR they did
// not submit — drives the is_approver capability flag on /me (nav tab visibility).
func (r *Repository) HasApprovableWork(ctx context.Context, callerID int64, hasLegal, hasSecurity bool) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM purchase_requests pr
			WHERE pr.requester_id <> $1 AND `+approvablePredicate+`)`,
		callerID, hasLegal, hasSecurity).Scan(&ok)
	return ok, err
}

// GetPurchaseRequest loads a request with its items, links, documents and requester.
func (r *Repository) GetPurchaseRequest(ctx context.Context, id int64) (*PurchaseRequest, error) {
	pr := &PurchaseRequest{}
	var email, name, reference pgtype.Text
	var details []byte
	err := r.pool.QueryRow(ctx, `
		SELECT pr.id, pr.reference, pr.title, pr.requester_id, pr.cost_center, pr.cost_center_id, pr.comments, pr.status,
		       pr.rejection_reason, pr.created_at, pr.updated_at, u.email, u.name,
		       pr.team, pr.entity, pr.category, pr.estimated_value, pr.currency,
		       pr.budget_approver_name, pr.budget_approver_email, pr.details
		FROM purchase_requests pr
		JOIN users u ON u.id = pr.requester_id
		WHERE pr.id = $1`, id).Scan(&pr.ID, &reference, &pr.Title, &pr.RequesterID, &pr.CostCenter, &pr.CostCenterID,
		&pr.Comments, &pr.Status, &pr.RejectionReason, &pr.CreatedAt, &pr.UpdatedAt, &email, &name,
		&pr.Team, &pr.Entity, &pr.Category, &pr.EstimatedValue, &pr.Currency,
		&pr.BudgetApproverName, &pr.BudgetApproverEmail, &details)
	if err != nil {
		return nil, err
	}
	if reference.Valid {
		pr.Reference = &reference.String
	}
	pr.Details = json.RawMessage(details)
	pr.Requester = &UserSummary{ID: pr.RequesterID, Email: email.String, Name: name.String}

	if pr.Items, err = r.listItems(ctx, id); err != nil {
		return nil, err
	}
	if pr.Links, err = r.listLinks(ctx, id); err != nil {
		return nil, err
	}
	if pr.Documents, err = r.ListDocuments(ctx, id); err != nil {
		return nil, err
	}
	if pr.Approvals, err = r.ListApprovals(ctx, id); err != nil {
		return nil, err
	}
	for _, a := range pr.Approvals {
		pr.ApprovalsTotal++
		if a.Status == model.PRApprovalApproved {
			pr.ApprovalsApproved++
		}
	}
	if pr.Recommendation, err = r.GetRecommendation(ctx, id); err != nil {
		return nil, err
	}
	return pr, nil
}

func (r *Repository) listItems(ctx context.Context, prID int64) ([]Item, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, description, quantity, position FROM pr_items
		WHERE purchase_request_id = $1 ORDER BY position, id`, prID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Item{}
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.ID, &it.Description, &it.Quantity, &it.Position); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

func (r *Repository) listLinks(ctx context.Context, prID int64) ([]Link, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, url, label, position FROM pr_links
		WHERE purchase_request_id = $1 ORDER BY position, id`, prID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	links := []Link{}
	for rows.Next() {
		var ln Link
		if err := rows.Scan(&ln.ID, &ln.URL, &ln.Label, &ln.Position); err != nil {
			return nil, err
		}
		links = append(links, ln)
	}
	return links, rows.Err()
}

// --- Documents (purchase-request-scoped wrappers) ---
//
// These preserve the Phase-1 handler API while delegating to the generic
// owner-scoped methods in procurement.go (owner_type = "purchase_request",
// owner_id = prID).

func (r *Repository) AddDocument(ctx context.Context, prID int64, filename, storedPath, contentType string, size int64, uploadedBy int64) (*Document, error) {
	return r.AddOwnedDocument(ctx, prID, ownerPurchaseRequest, prID, filename, storedPath, contentType, size, uploadedBy, "")
}

func (r *Repository) ListDocuments(ctx context.Context, prID int64) ([]Document, error) {
	return r.ListOwnedDocuments(ctx, ownerPurchaseRequest, prID)
}

// GetDocument fetches a single document scoped to its purchase request.
func (r *Repository) GetDocument(ctx context.Context, prID, docID int64) (*Document, error) {
	return r.GetOwnedDocument(ctx, ownerPurchaseRequest, prID, docID)
}

func (r *Repository) DeleteDocument(ctx context.Context, prID, docID int64) error {
	return r.DeleteOwnedDocument(ctx, ownerPurchaseRequest, prID, docID)
}
