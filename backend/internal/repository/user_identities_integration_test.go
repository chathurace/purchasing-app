package repository_test

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/cs/purchasing-app/internal/repository"
)

// One human, several OIDC subjects (migration 054). Asgardeo may issue an
// application-scoped `sub`, so the same person signing in through a second front
// end arrives with a subject this app has never seen. Before user_identities that
// login fell through to an INSERT that collided on users_email_lower_key — a 500
// on every request, locking the person out entirely. See docs/plans/21, Decision 5.

func emailFor(t *testing.T) string {
	t.Helper()
	return "identity-" + strings.ToLower(t.Name()) + "@example.com"
}

// The lockout scenario, end to end: an established user arrives with a second
// subject and must resolve to the SAME row rather than erroring or duplicating.
func TestSecondSubjectSameEmailAdoptsExistingUser(t *testing.T) {
	repo, ctx := newTestRepo(t)
	email := emailFor(t)
	subA := "sub-a-" + t.Name()
	subB := "sub-b-" + t.Name()

	first, linked, err := repo.ProvisionUserOnLogin(ctx, subA, email, "Real Person")
	if err != nil {
		t.Fatalf("first login: %v", err)
	}
	t.Cleanup(func() { _, _ = repo.Pool().Exec(ctx, `DELETE FROM users WHERE id=$1`, first.ID) })
	if linked {
		t.Error("a brand-new user should not report an identity link")
	}

	// The same person, second front end, different subject, same email claim.
	second, linked, err := repo.ProvisionUserOnLogin(ctx, subB, email, "Real Person")
	if err != nil {
		t.Fatalf("second subject was refused (this is the lockout bug): %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("second subject created a new user (%d) instead of adopting %d", second.ID, first.ID)
	}
	if !linked {
		t.Error("adopting an existing account should report linked=true, for the audit trail")
	}

	// Both subjects now resolve, with no further linking reported.
	for _, sub := range []string{subA, subB} {
		got, linked, err := repo.ProvisionUserOnLogin(ctx, sub, email, "Real Person")
		if err != nil {
			t.Fatalf("re-login with %s: %v", sub, err)
		}
		if got.ID != first.ID {
			t.Errorf("%s resolved to user %d, want %d", sub, got.ID, first.ID)
		}
		if linked {
			t.Errorf("%s reported a link on a known subject", sub)
		}
	}

	// users.sub keeps the FIRST subject — `pending` is derived from it, so it
	// must not start moving around.
	if first.Sub != subA {
		t.Errorf("users.sub = %q, want the first subject %q", first.Sub, subA)
	}
	var identities int
	if err := repo.Pool().QueryRow(ctx,
		`SELECT count(*) FROM user_identities WHERE user_id=$1`, first.ID).Scan(&identities); err != nil {
		t.Fatalf("count identities: %v", err)
	}
	if identities != 2 {
		t.Errorf("user holds %d identities, want 2", identities)
	}
}

// Case-insensitivity has to hold on the adoption path too — users_email_lower_key
// is case-insensitive, so a differently-cased email must adopt, not collide.
func TestSecondSubjectAdoptsAcrossEmailCase(t *testing.T) {
	repo, ctx := newTestRepo(t)
	email := emailFor(t)

	first, _, err := repo.ProvisionUserOnLogin(ctx, "sub-lower-"+t.Name(), email, "Person")
	if err != nil {
		t.Fatalf("first login: %v", err)
	}
	t.Cleanup(func() { _, _ = repo.Pool().Exec(ctx, `DELETE FROM users WHERE id=$1`, first.ID) })

	second, linked, err := repo.ProvisionUserOnLogin(ctx, "sub-upper-"+t.Name(), strings.ToUpper(email), "Person")
	if err != nil {
		t.Fatalf("second subject with upper-cased email: %v", err)
	}
	if second.ID != first.ID || !linked {
		t.Errorf("got user %d linked=%v, want %d linked=true", second.ID, linked, first.ID)
	}
}

// A first-ever login must record its subject, or every later request re-runs the
// cold path (and the advisory lock) instead of the cheap lookup.
func TestFirstLoginRecordsIdentity(t *testing.T) {
	repo, ctx := newTestRepo(t)
	sub := "sub-new-" + t.Name()

	u, _, err := repo.ProvisionUserOnLogin(ctx, sub, emailFor(t), "Newcomer")
	if err != nil {
		t.Fatalf("first login: %v", err)
	}
	t.Cleanup(func() { _, _ = repo.Pool().Exec(ctx, `DELETE FROM users WHERE id=$1`, u.ID) })

	var userID int64
	if err := repo.Pool().QueryRow(ctx,
		`SELECT user_id FROM user_identities WHERE sub=$1`, sub).Scan(&userID); err != nil {
		t.Fatalf("identity row missing after first login: %v", err)
	}
	if userID != u.ID {
		t.Errorf("identity points at user %d, want %d", userID, u.ID)
	}
}

// Claiming an admin invite must also record the identity — that path sets
// users.sub directly and could easily skip the new table.
func TestInviteClaimRecordsIdentity(t *testing.T) {
	repo, ctx := newTestRepo(t)
	email := emailFor(t)
	sub := "sub-invite-" + t.Name()

	invited, err := repo.CreateInvitedUser(ctx, email, "Invitee")
	if err != nil {
		t.Fatalf("create invited: %v", err)
	}
	t.Cleanup(func() { _, _ = repo.Pool().Exec(ctx, `DELETE FROM users WHERE id=$1`, invited.ID) })

	claimed, linked, err := repo.ProvisionUserOnLogin(ctx, sub, email, "Invitee Real")
	if err != nil {
		t.Fatalf("claim invite: %v", err)
	}
	if claimed.ID != invited.ID {
		t.Fatalf("claim created user %d instead of %d", claimed.ID, invited.ID)
	}
	if linked {
		t.Error("claiming a never-signed-in invite is not an identity link")
	}
	var userID int64
	if err := repo.Pool().QueryRow(ctx,
		`SELECT user_id FROM user_identities WHERE sub=$1`, sub).Scan(&userID); err != nil {
		t.Fatalf("identity row missing after invite claim: %v", err)
	}
	if userID != invited.ID {
		t.Errorf("identity points at user %d, want %d", userID, invited.ID)
	}
}

// Two front ends open at once, both hitting an unknown subject for the same
// person, each firing several parallel requests. Without the advisory lock these
// race between the email lookup and the insert and one loses on
// users_email_lower_key — a 500 at exactly the moment someone first opens the
// portal, which is the worst possible time for it.
func TestConcurrentUnknownSubjectsSameEmail(t *testing.T) {
	repo, ctx := newTestRepo(t)
	email := emailFor(t)

	const subs, perSub = 3, 4
	var wg sync.WaitGroup
	ids := make([]int64, subs*perSub)
	errs := make([]error, subs*perSub)

	for i := 0; i < subs; i++ {
		for j := 0; j < perSub; j++ {
			wg.Add(1)
			go func(slot, subIdx int) {
				defer wg.Done()
				u, _, err := repo.ProvisionUserOnLogin(ctx,
					"sub-concurrent-"+t.Name()+"-"+string(rune('a'+subIdx)), email, "Racer")
				if err != nil {
					errs[slot] = err
					return
				}
				ids[slot] = u.ID
			}(i*perSub+j, i)
		}
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent provision %d failed: %v", i, err)
		}
	}
	t.Cleanup(func() { _, _ = repo.Pool().Exec(ctx, `DELETE FROM users WHERE lower(email)=$1`, email) })

	// Every request must have landed on ONE user.
	for i, id := range ids {
		if id != ids[0] {
			t.Fatalf("request %d resolved to user %d, but request 0 got %d — duplicate accounts", i, id, ids[0])
		}
	}
	var rows int
	if err := repo.Pool().QueryRow(ctx,
		`SELECT count(*) FROM users WHERE lower(email)=$1`, email).Scan(&rows); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if rows != 1 {
		t.Errorf("%d user rows for one email, want 1", rows)
	}
	var identities int
	if err := repo.Pool().QueryRow(ctx,
		`SELECT count(*) FROM user_identities WHERE user_id=$1`, ids[0]).Scan(&identities); err != nil {
		t.Fatalf("count identities: %v", err)
	}
	if identities != subs {
		t.Errorf("recorded %d identities, want %d", identities, subs)
	}
}

// A back-channel logout names whichever subject the IdP knows. users.sub only
// holds the first one, so revocation has to resolve through user_identities or it
// silently no-ops for the other front end's subject.
func TestRevokeSessionsByEitherSubject(t *testing.T) {
	repo, ctx := newTestRepo(t)
	email := emailFor(t)
	subA := "sub-revoke-a-" + t.Name()
	subB := "sub-revoke-b-" + t.Name()

	u, _, err := repo.ProvisionUserOnLogin(ctx, subA, email, "Person")
	if err != nil {
		t.Fatalf("first login: %v", err)
	}
	t.Cleanup(func() { _, _ = repo.Pool().Exec(ctx, `DELETE FROM users WHERE id=$1`, u.ID) })
	if _, _, err := repo.ProvisionUserOnLogin(ctx, subB, email, "Person"); err != nil {
		t.Fatalf("second subject: %v", err)
	}

	for _, sub := range []string{subA, subB} {
		_, userID, err := repo.RevokeUserSessionsBySub(ctx, sub)
		if err != nil {
			t.Fatalf("revoke by %s: %v", sub, err)
		}
		if userID != u.ID {
			t.Errorf("revoke by %s resolved user %d, want %d", sub, userID, u.ID)
		}
	}

	// An unknown subject stays a no-op, not an error — the IdP broadcasts to
	// every registered app.
	revoked, userID, err := repo.RevokeUserSessionsBySub(ctx, "sub-nobody-"+t.Name())
	if err != nil || revoked != 0 || userID != 0 {
		t.Errorf("unknown subject: got (%d, %d, %v), want (0, 0, nil)", revoked, userID, err)
	}
}

// The guard rail. A token with no email-like claim must be REFUSED, not quietly
// turned into a second roleless account beside the person's real one. Resolving
// an existing user still needs no email, so the refusal is scoped to creation.
func TestNoEmailClaimRefusesCreationButNotResolution(t *testing.T) {
	repo, ctx := newTestRepo(t)
	sub := "sub-noemail-" + t.Name()

	// Creation with no email: refused.
	if _, _, err := repo.ProvisionUserOnLogin(ctx, sub, "", "No Email"); !errors.Is(err, repository.ErrNoEmailClaim) {
		t.Fatalf("expected ErrNoEmailClaim, got %v", err)
	}
	// Nothing was written — no half-created user, no orphan identity.
	var users, identities int
	if err := repo.Pool().QueryRow(ctx, `SELECT count(*) FROM users WHERE sub=$1`, sub).Scan(&users); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if err := repo.Pool().QueryRow(ctx, `SELECT count(*) FROM user_identities WHERE sub=$1`, sub).Scan(&identities); err != nil {
		t.Fatalf("count identities: %v", err)
	}
	if users != 0 || identities != 0 {
		t.Errorf("refused login still wrote rows: users=%d identities=%d", users, identities)
	}

	// An ESTABLISHED user, then a later request whose token has no email: still
	// resolves. Their subject is already recorded, so no email is needed — a
	// transient claim-config change must not lock out an existing user.
	email := emailFor(t)
	established, _, err := repo.ProvisionUserOnLogin(ctx, sub, email, "Real Person")
	if err != nil {
		t.Fatalf("create with email: %v", err)
	}
	t.Cleanup(func() { _, _ = repo.Pool().Exec(ctx, `DELETE FROM users WHERE id=$1`, established.ID) })

	again, linked, err := repo.ProvisionUserOnLogin(ctx, sub, "", "Real Person")
	if err != nil {
		t.Fatalf("established user refused when the email claim went missing: %v", err)
	}
	if again.ID != established.ID || linked {
		t.Errorf("got user %d linked=%v, want %d linked=false", again.ID, linked, established.ID)
	}
}
