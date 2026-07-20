package repository_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/cs/purchasing-app/internal/repository"
	"github.com/jackc/pgx/v5"
)

// TestInviteThenLoginLinksBySub covers the core user-management flow: an admin
// invites a user by email (no sub yet), and that person's first login claims the
// pending row by email instead of creating a duplicate.
func TestInviteThenLoginLinksBySub(t *testing.T) {
	repo, ctx := newTestRepo(t)
	email := "invitee-" + strings.ToLower(t.Name()) + "@example.com"

	invited, err := repo.CreateInvitedUser(ctx, email, "Invitee")
	if err != nil {
		t.Fatalf("create invited: %v", err)
	}
	t.Cleanup(func() { _, _ = repo.Pool().Exec(ctx, `DELETE FROM users WHERE id=$1`, invited.ID) })

	if invited.Sub != "" {
		t.Errorf("invited user should have no sub, got %q", invited.Sub)
	}
	if !invited.IsActive {
		t.Errorf("invited user should be active")
	}

	// Duplicate email (case-insensitive) is rejected.
	if _, err := repo.CreateInvitedUser(ctx, strings.ToUpper(email), "Dup"); !errors.Is(err, repository.ErrEmailExists) {
		t.Errorf("expected ErrEmailExists, got %v", err)
	}

	// First login with a differently-cased email claims the same row.
	sub := "oidc-sub-" + t.Name()
	linked, err := repo.ProvisionUserOnLogin(ctx, sub, strings.ToUpper(email), "Invitee Real")
	if err != nil {
		t.Fatalf("provision on login: %v", err)
	}
	if linked.ID != invited.ID {
		t.Fatalf("login created a new row (%d) instead of claiming invite (%d)", linked.ID, invited.ID)
	}
	if linked.Sub != sub {
		t.Errorf("sub not linked: got %q", linked.Sub)
	}

	// A second login with the same sub is idempotent (no new row).
	again, err := repo.ProvisionUserOnLogin(ctx, sub, email, "Invitee Real")
	if err != nil || again.ID != invited.ID {
		t.Fatalf("second login not idempotent: id=%d err=%v", again.ID, err)
	}
}

// TestUpdateInvitedUser covers editing a pending invite: it succeeds while the
// row has no sub, collides on a duplicate email, and is refused once the user has
// logged in (sub set).
func TestUpdateInvitedUser(t *testing.T) {
	repo, ctx := newTestRepo(t)
	base := strings.ToLower(t.Name())
	email := "invitee-" + base + "@example.com"

	u, err := repo.CreateInvitedUser(ctx, email, "Invitee")
	if err != nil {
		t.Fatalf("create invited: %v", err)
	}
	t.Cleanup(func() { _, _ = repo.Pool().Exec(ctx, `DELETE FROM users WHERE id=$1`, u.ID) })

	// Editing a pending user succeeds (email lowercased, name updated).
	if err := repo.UpdateInvitedUser(ctx, u.ID, "NEW-"+email, "New Name"); err != nil {
		t.Fatalf("update pending: %v", err)
	}
	users, err := repo.ListUsersWithRoles(ctx)
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	var got *repository.AdminUser
	for _, x := range users {
		if x.ID == u.ID {
			got = x
		}
	}
	if got == nil {
		t.Fatal("updated user missing from listing")
	}
	if got.Email != "new-"+email || got.Name != "New Name" {
		t.Errorf("update not applied: email=%q name=%q", got.Email, got.Name)
	}

	// A second invite whose email would collide with the edited one is rejected.
	other, err := repo.CreateInvitedUser(ctx, "other-"+base+"@example.com", "Other")
	if err != nil {
		t.Fatalf("create other: %v", err)
	}
	t.Cleanup(func() { _, _ = repo.Pool().Exec(ctx, `DELETE FROM users WHERE id=$1`, other.ID) })
	if err := repo.UpdateInvitedUser(ctx, other.ID, strings.ToUpper("new-"+email), "Other"); !errors.Is(err, repository.ErrEmailExists) {
		t.Errorf("expected ErrEmailExists, got %v", err)
	}

	// Unknown user → ErrNoRows.
	if err := repo.UpdateInvitedUser(ctx, -1, "nobody@example.com", ""); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("expected ErrNoRows for unknown user, got %v", err)
	}

	// Once the user logs in (sub set), editing is refused.
	if _, err := repo.ProvisionUserOnLogin(ctx, "oidc-sub-"+t.Name(), "new-"+email, "Invitee"); err != nil {
		t.Fatalf("provision on login: %v", err)
	}
	if err := repo.UpdateInvitedUser(ctx, u.ID, "changed-"+email, "Nope"); !errors.Is(err, repository.ErrUserAlreadyLoggedIn) {
		t.Errorf("expected ErrUserAlreadyLoggedIn, got %v", err)
	}
}

// TestRoleAndActiveManagement covers add/remove role, the last-admin guard, and
// deactivation.
func TestRoleAndActiveManagement(t *testing.T) {
	repo, ctx := newTestRepo(t)

	a, err := repo.CreateInvitedUser(ctx, "admin-a-"+strings.ToLower(t.Name())+"@example.com", "A")
	if err != nil {
		t.Fatalf("create a: %v", err)
	}
	b, err := repo.CreateInvitedUser(ctx, "admin-b-"+strings.ToLower(t.Name())+"@example.com", "B")
	if err != nil {
		t.Fatalf("create b: %v", err)
	}
	t.Cleanup(func() { _, _ = repo.Pool().Exec(ctx, `DELETE FROM users WHERE id IN ($1,$2)`, a.ID, b.ID) })

	if err := repo.EnsureUserHasRole(ctx, a.ID, "admin"); err != nil {
		t.Fatalf("grant admin a: %v", err)
	}

	// The dev DB may already hold admins, so assert relative to a baseline
	// rather than absolute counts. Capture whether *other* active admins exist
	// before b is involved.
	baseline, err := repo.OtherActiveAdminExists(ctx, a.ID)
	if err != nil {
		t.Fatalf("baseline: %v", err)
	}

	// Adding a second active admin guarantees another active admin exists.
	if err := repo.EnsureUserHasRole(ctx, b.ID, "admin"); err != nil {
		t.Fatalf("grant admin b: %v", err)
	}
	if other, err := repo.OtherActiveAdminExists(ctx, a.ID); err != nil || !other {
		t.Fatalf("expected another active admin: other=%v err=%v", other, err)
	}

	// Deactivating b removes its contribution: the result returns to baseline.
	if err := repo.SetUserActive(ctx, b.ID, false); err != nil {
		t.Fatalf("deactivate b: %v", err)
	}
	if other, err := repo.OtherActiveAdminExists(ctx, a.ID); err != nil || other != baseline {
		t.Fatalf("deactivated admin should not count: other=%v baseline=%v err=%v", other, baseline, err)
	}

	// Remove role round-trips.
	if err := repo.RemoveUserRole(ctx, b.ID, "admin"); err != nil {
		t.Fatalf("remove role: %v", err)
	}
	roles, err := repo.GetUserRoles(ctx, b.ID)
	if err != nil {
		t.Fatalf("get roles: %v", err)
	}
	for _, r := range roles {
		if r == "admin" {
			t.Errorf("admin role not removed: %v", roles)
		}
	}

	// b appears in the admin listing as deactivated.
	users, err := repo.ListUsersWithRoles(ctx)
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	var foundB bool
	for _, u := range users {
		if u.ID == b.ID {
			foundB = true
			if u.IsActive {
				t.Errorf("b should be deactivated in listing")
			}
			if !u.Pending {
				t.Errorf("b never logged in, should be pending")
			}
		}
	}
	if !foundB {
		t.Errorf("b missing from user listing")
	}
}
