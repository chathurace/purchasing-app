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
		// (3) Brand-new user. Upsert on sub so concurrent first-login requests
		// (the frontend fires several API calls in parallel, none of which find
		// the user by sub yet) don't collide on the users_sub_key unique
		// constraint — the losers take the DO UPDATE path instead of erroring.
		if !claimed {
			if err = tx.QueryRow(ctx, `
				INSERT INTO users (sub, email, name) VALUES ($1, $2, $3)
				ON CONFLICT (sub) DO UPDATE
					SET email = COALESCE($2, users.email),
					    name = COALESCE($3, users.name),
					    updated_at = NOW()
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

// ErrNotProcurementUser is returned when a PR assignee/collaborator does not hold
// the procurement role.
var ErrNotProcurementUser = errors.New("user is not a member of the procurement team")

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

// GetOrCreateUserByEmail returns the user matching email (case-insensitive),
// creating a pending invite (see CreateInvitedUser) when none exists. Used when a
// directory person picked for an id-based field (business-unit approver, team
// member) is not yet a provisioned app user; the row is claimed on their first
// login, exactly like an admin invite.
func (r *Repository) GetOrCreateUserByEmail(ctx context.Context, email, name string) (*User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	u, err := r.getUserByEmail(ctx, email)
	if err == nil {
		return u, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	created, err := r.CreateInvitedUser(ctx, email, strings.TrimSpace(name))
	if err != nil {
		// Lost a race with a concurrent create — re-read the existing row.
		if errors.Is(err, ErrEmailExists) {
			return r.getUserByEmail(ctx, email)
		}
		return nil, err
	}
	return created, nil
}

func (r *Repository) getUserByEmail(ctx context.Context, lowerEmail string) (*User, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, sub, email, name, is_active, created_at, updated_at
		FROM users WHERE lower(email) = $1`, lowerEmail)
	return scanUser(row)
}

// ErrUserAlreadyLoggedIn is returned when an admin tries to edit a user who has
// already claimed their account (has an OIDC sub). Their email is the login-match
// key at that point and is owned by the IdP, so it is no longer editable here.
var ErrUserAlreadyLoggedIn = errors.New("user has already logged in")

// UpdateInvitedUser edits the email/name of a pending invite (sub IS NULL) — a
// user an admin added but who has never signed in. Once claimed (sub set) the
// email is the login-match key and can't be changed; that case returns
// ErrUserAlreadyLoggedIn. A missing user returns pgx.ErrNoRows, and an email
// collision returns ErrEmailExists.
func (r *Repository) UpdateInvitedUser(ctx context.Context, id int64, email, name string) error {
	emailArg, nameArg := emailArgOf(email), nilIfEmpty(name)
	var updatedID int64
	err := r.pool.QueryRow(ctx, `
		UPDATE users SET email = $2, name = $3, updated_at = NOW()
		WHERE id = $1 AND sub IS NULL
		RETURNING id`, id, emailArg, nameArg).Scan(&updatedID)
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		// The row exists but has a sub (already logged in), or doesn't exist.
		var hasSub bool
		switch qerr := r.pool.QueryRow(ctx,
			`SELECT sub IS NOT NULL FROM users WHERE id = $1`, id).Scan(&hasSub); {
		case errors.Is(qerr, pgx.ErrNoRows):
			return pgx.ErrNoRows
		case qerr != nil:
			return qerr
		case hasSub:
			return ErrUserAlreadyLoggedIn
		default:
			return pgx.ErrNoRows
		}
	}
	var pgErr interface{ SQLState() string }
	if errors.As(err, &pgErr) && pgErr.SQLState() == "23505" {
		return ErrEmailExists
	}
	return err
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
	u.Sub = sub.String // empty for invited users who have never logged in
	u.Email = email.String
	u.Name = name.String
	return u, nil
}

// normEmail returns a lowercased, trimmed email as a plain string ("" when
// empty) — for NOT NULL columns (e.g. team_lead_email) and case-insensitive
// matching against the lowercased emails stored in the users table.
func normEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
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
	ID          int64  `json:"id"`
	Filename    string `json:"filename"`
	StoredPath  string `json:"-"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
	// Notes is an optional free-text note attached to the document (used by the
	// contract card for each draft/signed PDF).
	Notes      string    `json:"notes"`
	UploadedBy *int64    `json:"uploaded_by,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type PurchaseRequest struct {
	ID int64 `json:"id"`
	// Reference is the human-readable PR-YYYY-NNNNNNN identifier assigned at
	// submission. Nil for requests created before the feature (legacy display).
	Reference       *string `json:"reference"`
	Title           string  `json:"title"`
	RequesterID     int64   `json:"requester_id"`
	BusinessUnitID  *int64  `json:"business_unit_id"`
	Comments        string  `json:"comments"`
	Status          string  `json:"status"`
	RejectionReason string  `json:"rejection_reason"`
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
	// Team lead approval: the requester names a team lead by email who must
	// approve the PR before procurement can see or act on it. One decision per PR.
	TeamLeadEmail     string     `json:"team_lead_email"`
	TeamLeadStatus    string     `json:"team_lead_status"`
	TeamLeadNotes     string     `json:"team_lead_notes"`
	TeamLeadDecidedAt *time.Time `json:"team_lead_decided_at"`
	TeamLeadDecidedBy *int64     `json:"team_lead_decided_by"`
	// MyTeamLeadActionable is a per-caller display flag (detail reads only): true
	// when the caller may record the team lead decision (they are the team lead or
	// an admin). Not persisted; set by the handler like MyActionableTypes.
	MyTeamLeadActionable bool `json:"my_team_lead_actionable"`
	// PR assignment: once the team lead has approved, a procurement user must be
	// assigned before procurement work can start. Assignee + Collaborators are the
	// procurement users who may act on the PR. AssignedAt/AssignedBy stamp the
	// assignment. Collaborators is populated on detail reads.
	AssigneeID    *int64        `json:"assignee_id"`
	Assignee      *UserSummary  `json:"assignee,omitempty"`
	AssignedAt    *time.Time    `json:"assigned_at,omitempty"`
	Collaborators []UserSummary `json:"collaborators,omitempty"`
	// MyCanAssign / MyCanManageCollaborators / MyCanWork are per-caller display
	// flags (detail reads only), set by the handler — see the flag helpers there.
	MyCanAssign              bool       `json:"my_can_assign"`
	MyCanManageCollaborators bool       `json:"my_can_manage_collaborators"`
	MyCanWork                bool       `json:"my_can_work"`
	CreatedAt                time.Time  `json:"created_at"`
	UpdatedAt                time.Time  `json:"updated_at"`
	Items                    []Item     `json:"items"`
	Links                    []Link     `json:"links"`
	Documents                []Document `json:"documents"`
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
	Title string
	// BusinessUnitID links the request to a managed business unit (nullable). The
	// budget approver is picked from that unit's approver list.
	BusinessUnitID *int64
	Comments       string
	Items          []Item
	Links          []Link
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
	// TeamLeadEmail is the requester's team lead who must approve the PR. Stored
	// lowercased; honored on both create and update (an email change resets any
	// prior decision to pending — see UpdatePurchaseRequest).
	TeamLeadEmail string
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
			(title, requester_id, business_unit_id, comments,
			 team, entity, category, estimated_value, currency,
			 budget_approver_name, budget_approver_email, details, reference, team_lead_email)
		VALUES ($1, $2, $3, $4,
			 $5, $6, $7, $8, $9, $10, $11, $12::jsonb, $13, $14)
		RETURNING id`,
		in.Title, requesterID, in.BusinessUnitID, in.Comments,
		in.Team, in.Entity, in.Category, in.EstimatedValue, in.Currency,
		in.BudgetApproverName, in.BudgetApproverEmail, detailsOrEmpty(in.Details), reference,
		normEmail(in.TeamLeadEmail)).Scan(&id); err != nil {
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
// SetBudgetApprover updates only the PR's named budget-approver fields, leaving
// everything else (including team-lead decision state) untouched.
func (r *Repository) SetBudgetApprover(ctx context.Context, prID int64, name, email string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE purchase_requests
		SET budget_approver_name = $2, budget_approver_email = $3, updated_at = NOW()
		WHERE id = $1`, prID, name, email)
	return err
}

func (r *Repository) UpdatePurchaseRequest(ctx context.Context, id int64, in PurchaseRequestInput) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// A team-lead email change (or a currently-rejected decision) resets the
	// decision to pending so the (possibly new) team lead reviews the edited PR —
	// "edit & resubmit". The CASE predicates see the row's OLD values, so
	// `team_lead_email <> $13` compares the stored email to the incoming one.
	if _, err := tx.Exec(ctx, `
		UPDATE purchase_requests
		SET title = $2,
			business_unit_id = $3, comments = $4,
			team = $5, entity = $6, category = $7, estimated_value = $8, currency = $9,
			budget_approver_name = $10, budget_approver_email = $11, details = $12::jsonb,
			team_lead_status = CASE WHEN team_lead_status = 'rejected' OR team_lead_email <> $13
				THEN 'pending' ELSE team_lead_status END,
			team_lead_notes = CASE WHEN team_lead_status = 'rejected' OR team_lead_email <> $13
				THEN '' ELSE team_lead_notes END,
			team_lead_decided_at = CASE WHEN team_lead_status = 'rejected' OR team_lead_email <> $13
				THEN NULL ELSE team_lead_decided_at END,
			team_lead_decided_by = CASE WHEN team_lead_status = 'rejected' OR team_lead_email <> $13
				THEN NULL ELSE team_lead_decided_by END,
			team_lead_email = $13,
			updated_at = NOW()
		WHERE id = $1`,
		id, in.Title, in.BusinessUnitID, in.Comments,
		in.Team, in.Entity, in.Category, in.EstimatedValue, in.Currency,
		in.BudgetApproverName, in.BudgetApproverEmail, detailsOrEmpty(in.Details),
		normEmail(in.TeamLeadEmail)); err != nil {
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
	// PRScopeDefault preserves the legacy behavior: procurement/admin (seesAll) get
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

// PRListFilter holds the optional server-side filters applied by the
// Purchase-requests page controls. Zero-value fields (empty string / nil) mean
// "no filter on that field"; they are ANDed on top of the caller's scope
// visibility, so a filter can never widen what the caller may see.
type PRListFilter struct {
	// Status is an exact PR status match (e.g. "submitted"); "" = any status.
	Status string
	// BusinessUnitID matches pr.business_unit_id when non-nil.
	BusinessUnitID *int64
	// VendorID matches the PR's recommended vendor (pr_recommendations.vendor_id)
	// when non-nil.
	VendorID *int64
	// RequesterID matches pr.requester_id when non-nil.
	RequesterID *int64
	// AssigneeID matches pr.assignee_id when non-nil.
	AssigneeID *int64
}

// approvablePredicate is a SQL boolean fragment, true when the caller is an
// approver on the purchase request aliased `pr`: a named pr_approvals approver, a
// legal/security recommendation-card actor, the budget owner of the PR's budget
// unit, or a named budget-chain step approver (email match). It binds $1 (caller
// user id), $2 (hasLegal), $3 (hasSecurity), $4 (caller email, lowercased);
// callers MUST supply those four args in that order.
// budgetEmailMatch is a SQL boolean fragment, true when the lowercased caller
// email ($4) is a member of the PR's free-text, comma-separated
// budget_approver_email list (case-insensitive; empty segments ignored). This is
// the approver the requester picked from the business unit's approver list.
const budgetEmailMatch = `($4 <> '' AND $4 = ANY (
	SELECT lower(trim(e)) FROM unnest(string_to_array(pr.budget_approver_email, ',')) AS e WHERE trim(e) <> ''))`

const approvablePredicate = `(
	EXISTS (SELECT 1 FROM pr_approvals a WHERE a.purchase_request_id = pr.id AND a.approver_id = $1)
	OR EXISTS (
		SELECT 1 FROM pr_recommendation_approvals ra
		JOIN pr_recommendations rec ON rec.id = ra.recommendation_id
		WHERE rec.purchase_request_id = pr.id AND (
			(ra.approval_type = 'legal'    AND $2)
			OR (ra.approval_type = 'security' AND $3)
			OR (ra.approval_type = 'budget' AND ` + budgetEmailMatch + `)
		))
	OR EXISTS (
		SELECT 1 FROM pr_recommendation_budget_steps s
		JOIN pr_recommendations recs ON recs.id = s.recommendation_id
		WHERE recs.purchase_request_id = pr.id AND s.approver_email <> '' AND lower(s.approver_email) = $4))`

// teamLeadMatch is a SQL boolean fragment, true when the caller is the named team
// lead of the PR aliased `pr` — a case-insensitive match of the PR's team lead
// email against the caller's email. Binds $4 (caller email, lowercased); the
// empty-string guard means a PR with no team lead never matches an empty caller.
const teamLeadMatch = `(pr.team_lead_email <> '' AND pr.team_lead_email = $4)`

// myApprovalStateExpr is a SQL scalar expression resolving the caller's unified
// approval state across all three systems: 'pending' if the caller's team-lead
// decision is pending, any named row is pending, or an actionable card is still
// outstanding; else 'rejected' if the caller rejected as team lead or on a named
// row (cards cannot be rejected); else 'approved'. Binds $1/$2/$3/$4 as above.
// Only meaningful for PRs the caller actually approves; harmless ('approved')
// otherwise.
const myApprovalStateExpr = `
	CASE
		WHEN (` + teamLeadMatch + ` AND pr.team_lead_status = 'pending')
		  OR EXISTS (SELECT 1 FROM pr_approvals a WHERE a.purchase_request_id = pr.id AND a.approver_id = $1 AND a.status = 'pending')
		  OR EXISTS (
			SELECT 1 FROM pr_recommendation_approvals ra
			JOIN pr_recommendations rec ON rec.id = ra.recommendation_id
			WHERE rec.purchase_request_id = pr.id AND ra.approved_by IS NULL AND (
				(ra.approval_type = 'legal'    AND $2)
				OR (ra.approval_type = 'security' AND $3)
				OR (ra.approval_type = 'budget' AND ` + budgetEmailMatch + `)))
		  OR EXISTS (
			SELECT 1 FROM pr_recommendation_budget_steps s
			JOIN pr_recommendations recs ON recs.id = s.recommendation_id
			WHERE recs.purchase_request_id = pr.id AND lower(s.approver_email) = $4 AND s.decision = 'pending')
		THEN 'pending'
		WHEN (` + teamLeadMatch + ` AND pr.team_lead_status = 'rejected')
		  OR EXISTS (SELECT 1 FROM pr_approvals a WHERE a.purchase_request_id = pr.id AND a.approver_id = $1 AND a.status = 'rejected')
		  OR EXISTS (
			SELECT 1 FROM pr_recommendation_budget_steps s
			JOIN pr_recommendations recs ON recs.id = s.recommendation_id
			WHERE recs.purchase_request_id = pr.id AND lower(s.approver_email) = $4 AND s.decision = 'rejected')
		THEN 'rejected'
		ELSE 'approved'
	END`

// ListPurchaseRequests returns request summaries (no items/links/documents) with
// an approval tally, the caller's own approval status, and their unified approval
// state on each. callerID/callerEmail/hasLegal/hasSecurity resolve those
// per-caller fields; scope selects which rows are returned (see PRListScope).
// seesAll/isAdmin only apply to PRScopeDefault.
//
// Visibility gate: a PR is broadly visible only once its team lead has approved
// it. Before that only the requester, the team lead (email match), and admins may
// see it — procurement/named-approvers/recommendation-card actors are excluded.
func (r *Repository) ListPurchaseRequests(ctx context.Context, callerID int64, callerEmail string, seesAll, isAdmin, hasLegal, hasSecurity bool, scope PRListScope, filter PRListFilter) ([]*PurchaseRequest, error) {
	query := `
		SELECT pr.id, pr.reference, pr.title, pr.requester_id, pr.business_unit_id, pr.comments, pr.status,
		       pr.rejection_reason, pr.created_at, pr.updated_at, u.email, u.name,
		       pr.team, pr.entity, pr.category, pr.estimated_value, pr.currency,
		       pr.team_lead_email, pr.team_lead_status,
		       pr.assignee_id, au.email, au.name,
		       (SELECT count(*) FROM pr_approvals a WHERE a.purchase_request_id = pr.id) AS approvals_total,
		       (SELECT count(*) FROM pr_approvals a WHERE a.purchase_request_id = pr.id AND a.status = 'approved') AS approvals_approved,
		       COALESCE(
		           (SELECT a.status FROM pr_approvals a WHERE a.purchase_request_id = pr.id AND a.approver_id = $1),
		           CASE WHEN ` + teamLeadMatch + ` THEN pr.team_lead_status ELSE NULL END
		       ) AS my_approval_status,
		       (` + myApprovalStateExpr + `) AS my_approval_state
		FROM purchase_requests pr
		JOIN users u ON u.id = pr.requester_id
		LEFT JOIN users au ON au.id = pr.assignee_id`
	// $1/$2/$3/$4 are always bound because myApprovalStateExpr references all four.
	args := []any{callerID, hasLegal, hasSecurity, normEmail(callerEmail)}

	// wheres accumulates the caller's scope visibility plus any page filters; they
	// are ANDed, so filters only ever narrow what the caller may already see.
	var wheres []string
	switch scope {
	case PRScopeMine:
		wheres = append(wheres, `pr.requester_id = $1`)
	case PRScopeApprovals:
		// Team lead sees their own queue at any status; other card actors only
		// after the team lead has approved (before that the PR is hidden).
		wheres = append(wheres, `pr.requester_id <> $1 AND (`+teamLeadMatch+
			` OR (pr.team_lead_status = 'approved' AND `+approvablePredicate+`))`)
	default:
		switch {
		case isAdmin:
			// Admins see every PR at any status — no scope restriction.
		case seesAll:
			// Procurement (non-admin): own + team-lead-of + any team-lead-approved PR.
			wheres = append(wheres, `(pr.requester_id = $1 OR `+teamLeadMatch+
				` OR pr.team_lead_status = 'approved')`)
		default:
			wheres = append(wheres, `(pr.requester_id = $1 OR `+teamLeadMatch+
				` OR (pr.team_lead_status = 'approved' AND `+approvablePredicate+`))`)
		}
	}

	// Optional server-side filters (Purchase-requests page controls).
	if filter.Status != "" {
		args = append(args, filter.Status)
		wheres = append(wheres, fmt.Sprintf(`pr.status = $%d`, len(args)))
	}
	if filter.BusinessUnitID != nil {
		args = append(args, *filter.BusinessUnitID)
		wheres = append(wheres, fmt.Sprintf(`pr.business_unit_id = $%d`, len(args)))
	}
	if filter.RequesterID != nil {
		args = append(args, *filter.RequesterID)
		wheres = append(wheres, fmt.Sprintf(`pr.requester_id = $%d`, len(args)))
	}
	if filter.AssigneeID != nil {
		args = append(args, *filter.AssigneeID)
		wheres = append(wheres, fmt.Sprintf(`pr.assignee_id = $%d`, len(args)))
	}
	if filter.VendorID != nil {
		args = append(args, *filter.VendorID)
		wheres = append(wheres, fmt.Sprintf(
			`EXISTS (SELECT 1 FROM pr_recommendations rec WHERE rec.purchase_request_id = pr.id AND rec.vendor_id = $%d)`, len(args)))
	}

	if len(wheres) > 0 {
		query += ` WHERE ` + strings.Join(wheres, ` AND `)
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
		var assigneeID pgtype.Int8
		var assigneeEmail, assigneeName pgtype.Text
		if err := rows.Scan(&pr.ID, &reference, &pr.Title, &pr.RequesterID, &pr.BusinessUnitID, &pr.Comments,
			&pr.Status, &pr.RejectionReason, &pr.CreatedAt, &pr.UpdatedAt, &email, &name,
			&pr.Team, &pr.Entity, &pr.Category, &pr.EstimatedValue, &pr.Currency,
			&pr.TeamLeadEmail, &pr.TeamLeadStatus,
			&assigneeID, &assigneeEmail, &assigneeName,
			&pr.ApprovalsTotal, &pr.ApprovalsApproved, &myStatus, &myState); err != nil {
			return nil, err
		}
		if reference.Valid {
			pr.Reference = &reference.String
		}
		if assigneeID.Valid {
			v := assigneeID.Int64
			pr.AssigneeID = &v
			pr.Assignee = &UserSummary{ID: v, Email: assigneeEmail.String, Name: assigneeName.String}
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
func (r *Repository) IsApproverForPR(ctx context.Context, prID, callerID int64, callerEmail string, hasLegal, hasSecurity bool) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM purchase_requests pr
			WHERE pr.id = $5 AND `+approvablePredicate+`)`,
		callerID, hasLegal, hasSecurity, normEmail(callerEmail), prID).Scan(&ok)
	return ok, err
}

// HasApprovableWork reports whether the caller is an approver on any PR they did
// not submit — drives the is_approver capability flag on /me (nav tab visibility).
// A team lead always counts (their queue is visible pre-approval); other card
// actors count only once the team lead has approved, matching the Approvals-tab
// scope in ListPurchaseRequests.
func (r *Repository) HasApprovableWork(ctx context.Context, callerID int64, callerEmail string, hasLegal, hasSecurity bool) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM purchase_requests pr
			WHERE pr.requester_id <> $1 AND (`+teamLeadMatch+`
				OR (pr.team_lead_status = 'approved' AND `+approvablePredicate+`)))`,
		callerID, hasLegal, hasSecurity, normEmail(callerEmail)).Scan(&ok)
	return ok, err
}

// RecordTeamLeadDecision sets the PR's team lead decision (approve|reject) with
// notes, stamping who decided and when. Allowed from any current status so the
// team lead can revise a prior decision. Returns ErrInvalidState if the PR is gone.
func (r *Repository) RecordTeamLeadDecision(ctx context.Context, prID, deciderID int64, decision, notes string) error {
	ct, err := r.pool.Exec(ctx, `
		UPDATE purchase_requests
		SET team_lead_status = $2, team_lead_notes = $3,
			team_lead_decided_at = NOW(), team_lead_decided_by = $4, updated_at = NOW()
		WHERE id = $1`, prID, decision, notes, deciderID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrInvalidState
	}
	return nil
}

// ResetTeamLeadToPending clears a PR's team lead decision — status back to
// pending, notes blanked, decided stamp removed — so it can be reconsidered
// ("edit decision" → move to pending). ErrInvalidState if the PR is gone.
func (r *Repository) ResetTeamLeadToPending(ctx context.Context, prID int64) error {
	ct, err := r.pool.Exec(ctx, `
		UPDATE purchase_requests
		SET team_lead_status = $2, team_lead_notes = '',
			team_lead_decided_at = NULL, team_lead_decided_by = NULL, updated_at = NOW()
		WHERE id = $1`, prID, model.TeamLeadPending)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrInvalidState
	}
	return nil
}

// UpdateTeamLeadEmail changes a PR's team lead email (normalized). Used for the
// inline edit on the approval card while approval is still pending; the decision
// fields are untouched (they are already empty at pending). ErrInvalidState if the
// PR is gone.
func (r *Repository) UpdateTeamLeadEmail(ctx context.Context, prID int64, email string) error {
	ct, err := r.pool.Exec(ctx, `
		UPDATE purchase_requests
		SET team_lead_email = $2, updated_at = NOW()
		WHERE id = $1`, prID, normEmail(email))
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrInvalidState
	}
	return nil
}

// CanBeAssignedPR reports whether the user may be a PR assignee/collaborator — they
// hold procurement, procurement_admin, or admin. (Membership of the Procurement team
// is the plain procurement role, but procurement_admins/admins do procurement work
// too and must be assignable, e.g. a procurement_admin claiming a PR for themselves.)
func (r *Repository) CanBeAssignedPR(ctx context.Context, userID int64) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM user_roles ur
			JOIN roles rl ON rl.id = ur.role_id
			WHERE ur.user_id = $1 AND rl.name = ANY($2))`,
		userID, []string{model.RoleProcurement, model.RoleProcurementAdmin, model.RoleAdmin}).Scan(&ok)
	return ok, err
}

// SetPRAssignee sets (or clears, when assigneeID is nil) the PR's procurement
// assignee, stamping who assigned and when. A non-nil assignee must be able to work
// on procurement (see CanBeAssignedPR), else ErrNotProcurementUser. ErrInvalidState
// if the PR is gone.
func (r *Repository) SetPRAssignee(ctx context.Context, prID int64, assigneeID *int64, actorID int64) error {
	if assigneeID != nil {
		ok, err := r.CanBeAssignedPR(ctx, *assigneeID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrNotProcurementUser
		}
	}
	var assignedAt any
	var assignedBy any
	if assigneeID != nil {
		assignedAt = time.Now()
		assignedBy = actorID
	}
	ct, err := r.pool.Exec(ctx, `
		UPDATE purchase_requests
		SET assignee_id = $2, assigned_at = $3, assigned_by = $4, updated_at = NOW()
		WHERE id = $1`, prID, assigneeID, assignedAt, assignedBy)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrInvalidState
	}
	return nil
}

// AddPRCollaborator adds a procurement user as a collaborator on the PR (idempotent).
// The user must be able to work on procurement (see CanBeAssignedPR), else
// ErrNotProcurementUser.
func (r *Repository) AddPRCollaborator(ctx context.Context, prID, userID, actorID int64) error {
	ok, err := r.CanBeAssignedPR(ctx, userID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotProcurementUser
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO purchase_request_collaborators (purchase_request_id, user_id, added_by)
		VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, prID, userID, actorID)
	return err
}

// EnsurePRCollaborator records userID as a collaborator on the PR unless they are
// already the assignee or already a collaborator (self-added). Returns whether a new
// row was inserted, so the caller records a single event only when it actually adds
// them. Idempotent; the caller is responsible for confirming the user is a
// procurement actor.
func (r *Repository) EnsurePRCollaborator(ctx context.Context, prID, userID int64) (bool, error) {
	ct, err := r.pool.Exec(ctx, `
		INSERT INTO purchase_request_collaborators (purchase_request_id, user_id, added_by)
		SELECT $1, $2, $2
		WHERE NOT EXISTS (
			SELECT 1 FROM purchase_requests WHERE id = $1 AND assignee_id = $2)
		ON CONFLICT DO NOTHING`, prID, userID)
	if err != nil {
		return false, err
	}
	return ct.RowsAffected() > 0, nil
}

// RemovePRCollaborator removes a collaborator from the PR (no-op if absent).
func (r *Repository) RemovePRCollaborator(ctx context.Context, prID, userID int64) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM purchase_request_collaborators
		WHERE purchase_request_id = $1 AND user_id = $2`, prID, userID)
	return err
}

// listCollaborators returns the PR's collaborator users (id/email/name), ordered
// by email.
func (r *Repository) listCollaborators(ctx context.Context, prID int64) ([]UserSummary, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT u.id, u.email, u.name
		FROM purchase_request_collaborators c
		JOIN users u ON u.id = c.user_id
		WHERE c.purchase_request_id = $1
		ORDER BY lower(u.email), u.id`, prID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UserSummary{}
	for rows.Next() {
		var u UserSummary
		var email, name pgtype.Text
		if err := rows.Scan(&u.ID, &email, &name); err != nil {
			return nil, err
		}
		u.Email = email.String
		u.Name = name.String
		out = append(out, u)
	}
	return out, rows.Err()
}

// GetPurchaseRequest loads a request with its items, links, documents and requester.
func (r *Repository) GetPurchaseRequest(ctx context.Context, id int64) (*PurchaseRequest, error) {
	pr := &PurchaseRequest{}
	var email, name, reference pgtype.Text
	var details []byte
	var tlDecidedAt pgtype.Timestamptz
	var tlDecidedBy pgtype.Int8
	var assigneeID pgtype.Int8
	var assignedAt pgtype.Timestamptz
	var assigneeEmail, assigneeName pgtype.Text
	err := r.pool.QueryRow(ctx, `
		SELECT pr.id, pr.reference, pr.title, pr.requester_id, pr.business_unit_id, pr.comments, pr.status,
		       pr.rejection_reason, pr.created_at, pr.updated_at, u.email, u.name,
		       pr.team, pr.entity, pr.category, pr.estimated_value, pr.currency,
		       pr.budget_approver_name, pr.budget_approver_email, pr.details,
		       pr.team_lead_email, pr.team_lead_status, pr.team_lead_notes,
		       pr.team_lead_decided_at, pr.team_lead_decided_by,
		       pr.assignee_id, pr.assigned_at, au.email, au.name
		FROM purchase_requests pr
		JOIN users u ON u.id = pr.requester_id
		LEFT JOIN users au ON au.id = pr.assignee_id
		WHERE pr.id = $1`, id).Scan(&pr.ID, &reference, &pr.Title, &pr.RequesterID, &pr.BusinessUnitID,
		&pr.Comments, &pr.Status, &pr.RejectionReason, &pr.CreatedAt, &pr.UpdatedAt, &email, &name,
		&pr.Team, &pr.Entity, &pr.Category, &pr.EstimatedValue, &pr.Currency,
		&pr.BudgetApproverName, &pr.BudgetApproverEmail, &details,
		&pr.TeamLeadEmail, &pr.TeamLeadStatus, &pr.TeamLeadNotes, &tlDecidedAt, &tlDecidedBy,
		&assigneeID, &assignedAt, &assigneeEmail, &assigneeName)
	if err != nil {
		return nil, err
	}
	if assigneeID.Valid {
		v := assigneeID.Int64
		pr.AssigneeID = &v
		pr.Assignee = &UserSummary{ID: v, Email: assigneeEmail.String, Name: assigneeName.String}
	}
	if assignedAt.Valid {
		t := assignedAt.Time
		pr.AssignedAt = &t
	}
	if tlDecidedAt.Valid {
		t := tlDecidedAt.Time
		pr.TeamLeadDecidedAt = &t
	}
	if tlDecidedBy.Valid {
		v := tlDecidedBy.Int64
		pr.TeamLeadDecidedBy = &v
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
	if pr.Collaborators, err = r.listCollaborators(ctx, id); err != nil {
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
