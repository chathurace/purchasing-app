package repository

import (
	"context"
	"time"

	"github.com/cs/purchasing-app/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// =====================================================================
// Teams
// =====================================================================

// Team is a named group (Legal / Security / Procurement) whose membership is a role:
// a user is a member iff they hold member_role, so Members is derived, not stored.
// team_email is a shared address CC'd on the team's notifications.
//
// AdminMembers is the (derived) set of users holding the team's *admin* role variant,
// if it has one — currently only the Procurement team, whose admins hold
// procurement_admin. They count as part of the team for display, but the base
// Members list (and everything keyed on it, e.g. the assignee pool) stays scoped to
// plain member_role holders; admins are granted/revoked from the Users page, not here.
type Team struct {
	ID           int64         `json:"id"`
	Key          string        `json:"key"`
	Name         string        `json:"name"`
	MemberRole   string        `json:"member_role"`
	TeamEmail    string        `json:"team_email"`
	Members      []UserSummary `json:"members"`
	AdminMembers []UserSummary `json:"admin_members"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

// adminRoleForMemberRole returns the admin-role variant that also counts as a member
// of a team with the given member_role, or "" when the role has no admin variant.
func adminRoleForMemberRole(memberRole string) string {
	if memberRole == model.RoleProcurement {
		return model.RoleProcurementAdmin
	}
	return ""
}

// loadMembers fills a team's Members (base member_role holders) and, when the role
// has an admin variant, its AdminMembers.
func (r *Repository) loadMembers(ctx context.Context, t *Team) error {
	members, err := r.ListUsersByRole(ctx, t.MemberRole)
	if err != nil {
		return err
	}
	t.Members = members
	t.AdminMembers = []UserSummary{}
	if adminRole := adminRoleForMemberRole(t.MemberRole); adminRole != "" {
		admins, err := r.ListUsersByRole(ctx, adminRole)
		if err != nil {
			return err
		}
		t.AdminMembers = admins
	}
	return nil
}

// ListTeams returns every team with its (active) members attached.
func (r *Repository) ListTeams(ctx context.Context) ([]*Team, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, key, name, member_role, team_email, created_at, updated_at
		FROM teams ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Team
	for rows.Next() {
		t := &Team{Members: []UserSummary{}}
		if err := rows.Scan(&t.ID, &t.Key, &t.Name, &t.MemberRole, &t.TeamEmail, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, t := range out {
		if err := r.loadMembers(ctx, t); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// GetTeamByKey loads one team by its stable key (pgx.ErrNoRows if unknown),
// including its members.
func (r *Repository) GetTeamByKey(ctx context.Context, key string) (*Team, error) {
	t := &Team{Members: []UserSummary{}}
	err := r.pool.QueryRow(ctx, `
		SELECT id, key, name, member_role, team_email, created_at, updated_at
		FROM teams WHERE key = $1`, key).
		Scan(&t.ID, &t.Key, &t.Name, &t.MemberRole, &t.TeamEmail, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if err := r.loadMembers(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// TeamEmailForRole returns the team_email of the team whose member_role matches
// the given role, or "" when there is no such team (or its email is unset). Used
// to CC a team on its assignee notifications.
func (r *Repository) TeamEmailForRole(ctx context.Context, role string) (string, error) {
	var email string
	err := r.pool.QueryRow(ctx, `SELECT team_email FROM teams WHERE member_role = $1`, role).Scan(&email)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", nil
		}
		return "", err
	}
	return email, nil
}

// SetTeamEmail updates a team's shared email address (pgx.ErrNoRows if the key is
// unknown).
func (r *Repository) SetTeamEmail(ctx context.Context, key, email string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE teams SET team_email = $2, updated_at = NOW() WHERE key = $1`, key, email)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// ListUsersByRole returns the active users holding the given role (id/email/name),
// ordered by email — the members of a team and the pool a card may be assigned to.
func (r *Repository) ListUsersByRole(ctx context.Context, role string) ([]UserSummary, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT u.id, u.email, u.name
		FROM users u
		JOIN user_roles ur ON ur.user_id = u.id
		JOIN roles rl ON rl.id = ur.role_id
		WHERE rl.name = $1 AND u.is_active
		ORDER BY lower(u.email), u.id`, role)
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

// UserHasRole reports whether the user currently holds the given role. Used to
// validate an assignee is a member of the card's team.
func (r *Repository) UserHasRole(ctx context.Context, userID int64, role string) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM user_roles ur
			JOIN roles rl ON rl.id = ur.role_id
			WHERE ur.user_id = $1 AND rl.name = $2)`, userID, role).Scan(&ok)
	return ok, err
}
