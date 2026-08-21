package offboard

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/cs/purchasing-app/internal/directory"
	"github.com/cs/purchasing-app/internal/model"
	"github.com/cs/purchasing-app/internal/repository"
	"github.com/rs/zerolog"
)

func idpUser(email string, active bool) directory.User {
	return directory.User{Name: email, Email: email, Active: active}
}

func TestDecideSignsOutMissingAndDisabled(t *testing.T) {
	candidates := []Candidate{
		{UserID: 1, Email: "stays@example.com"},
		{UserID: 2, Email: "deleted@example.com"},
		{UserID: 3, Email: "disabled@example.com"},
		// Email case and surrounding space must not matter — the app lowercases
		// on write, but a directory may not.
		{UserID: 4, Email: "  Mixed.Case@Example.com "},
	}
	snapshot := []directory.User{
		idpUser("stays@example.com", true),
		idpUser("disabled@example.com", false),
		idpUser("mixed.case@example.com", true),
		idpUser("someone.else@example.com", true),
	}

	targets, err := Decide(candidates, snapshot)
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	got := map[int64]Reason{}
	for _, tg := range targets {
		got[tg.UserID] = tg.Reason
	}
	if len(got) != 2 {
		t.Fatalf("signed out %v, want exactly users 2 and 3", got)
	}
	if got[2] != ReasonMissing {
		t.Errorf("deleted user reason = %q, want %q", got[2], ReasonMissing)
	}
	if got[3] != ReasonDisabled {
		t.Errorf("disabled user reason = %q, want %q", got[3], ReasonDisabled)
	}
}

// The guards are the point of this package: a bad snapshot must produce no
// action at all, never a mass sign-out.
func TestDecideRefusesUntrustworthySnapshots(t *testing.T) {
	many := func(n int) []Candidate {
		out := make([]Candidate, 0, n)
		for i := 0; i < n; i++ {
			out = append(out, Candidate{UserID: int64(i + 1), Email: fmt.Sprintf("user%d@example.com", i)})
		}
		return out
	}

	t.Run("empty snapshot", func(t *testing.T) {
		// "The fetch returned nothing" is indistinguishable from "everyone was
		// deleted", so it must never be acted on.
		if _, err := Decide(many(3), nil); err == nil {
			t.Fatal("an empty snapshot was accepted")
		}
	})

	t.Run("snapshot with no usable emails", func(t *testing.T) {
		snapshot := []directory.User{{Name: "no email", Email: "", Active: true}}
		if _, err := Decide(many(3), snapshot); err == nil {
			t.Fatal("a snapshot with no emails was accepted")
		}
	})

	t.Run("wrong tenant would sign out nearly everyone", func(t *testing.T) {
		// 10 signed-in users, a directory that knows none of them.
		snapshot := []directory.User{idpUser("stranger@other-tenant.com", true)}
		if _, err := Decide(many(10), snapshot); err == nil {
			t.Fatal("a run that would sign out every user was accepted")
		}
	})

	t.Run("small numbers are not blocked by the ratio", func(t *testing.T) {
		// 2 of 3 leaving on the same day is ordinary, not a misconfiguration.
		candidates := many(3)
		snapshot := []directory.User{idpUser("user0@example.com", true)}
		targets, err := Decide(candidates, snapshot)
		if err != nil {
			t.Fatalf("Decide: %v", err)
		}
		if len(targets) != 2 {
			t.Fatalf("signed out %d, want 2", len(targets))
		}
	})

	t.Run("no candidates is not an error", func(t *testing.T) {
		targets, err := Decide(nil, []directory.User{idpUser("a@example.com", true)})
		if err != nil || targets != nil {
			t.Fatalf("got (%v, %v), want (nil, nil)", targets, err)
		}
	})
}

// A duplicated identity across user stores must not sign out a working account.
func TestDecidePrefersTheActiveDuplicate(t *testing.T) {
	candidates := []Candidate{{UserID: 1, Email: "dup@example.com"}}
	for _, order := range [][]directory.User{
		{idpUser("dup@example.com", false), idpUser("dup@example.com", true)},
		{idpUser("dup@example.com", true), idpUser("dup@example.com", false)},
	} {
		targets, err := Decide(candidates, append(order, idpUser("filler@example.com", true)))
		if err != nil {
			t.Fatalf("Decide: %v", err)
		}
		if len(targets) != 0 {
			t.Fatalf("signed out a user who has an active directory entry: %+v", targets)
		}
	}
}

// --- sweeper wiring ----------------------------------------------------------

type fakeStore struct {
	candidates []repository.UserSummary
	listErr    error
	revoked    []int64
	audits     []string
}

func (f *fakeStore) UsersWithLiveSessions(context.Context) ([]repository.UserSummary, error) {
	return f.candidates, f.listErr
}

func (f *fakeStore) RevokeAllUserSessions(_ context.Context, userID int64) (int64, error) {
	f.revoked = append(f.revoked, userID)
	return 1, nil
}

func (f *fakeStore) AddAuditEvent(_ context.Context, action, qualifier, entityType string, entityID *int64, _ string, actorID int64, actorEmail string) error {
	if actorID != 0 || actorEmail != "" {
		return fmt.Errorf("sweep audit must have no actor, got id=%d email=%q", actorID, actorEmail)
	}
	f.audits = append(f.audits, fmt.Sprintf("%s/%s/%s/%d", action, qualifier, entityType, *entityID))
	return nil
}

type fakeDir struct {
	users   []directory.User
	enabled bool
	err     error
}

func (f *fakeDir) Enabled() bool { return f.enabled }
func (f *fakeDir) List(context.Context, bool) ([]directory.User, error) {
	return f.users, f.err
}

func quietLogger() zerolog.Logger { return zerolog.New(io.Discard) }

func newSweepFixture(dryRun bool) (*Sweeper, *fakeStore) {
	store := &fakeStore{candidates: []repository.UserSummary{
		{ID: 1, Email: "stays@example.com"},
		{ID: 2, Email: "deleted@example.com"},
	}}
	dir := &fakeDir{enabled: true, users: []directory.User{
		idpUser("stays@example.com", true),
		idpUser("other@example.com", true),
	}}
	return New(store, dir, Config{DryRun: dryRun}, quietLogger()), store
}

func TestSweepRevokesAndAudits(t *testing.T) {
	sweeper, store := newSweepFixture(false)
	sweeper.Sweep(context.Background())

	if len(store.revoked) != 1 || store.revoked[0] != 2 {
		t.Fatalf("revoked %v, want just user 2", store.revoked)
	}
	want := fmt.Sprintf("%s/%s/%s/2", model.AuditRevokeUserSessions, model.QualifierIdPOffboard, model.EntityUser)
	if len(store.audits) != 1 || store.audits[0] != want {
		t.Fatalf("audits %v, want [%s]", store.audits, want)
	}
}

func TestSweepDryRunRevokesNothing(t *testing.T) {
	sweeper, store := newSweepFixture(true)
	sweeper.Sweep(context.Background())

	if len(store.revoked) != 0 || len(store.audits) != 0 {
		t.Fatalf("dry run acted: revoked=%v audits=%v", store.revoked, store.audits)
	}
}

// A directory read failure must leave every session alone — the alternative is
// signing out the whole company because SCIM had a bad minute.
func TestSweepDoesNothingWhenTheDirectoryFails(t *testing.T) {
	store := &fakeStore{candidates: []repository.UserSummary{{ID: 1, Email: "a@example.com"}}}
	sweeper := New(store, &fakeDir{enabled: true, err: errors.New("scim down")}, Config{}, quietLogger())
	sweeper.Sweep(context.Background())
	if len(store.revoked) != 0 {
		t.Fatalf("revoked %v after a directory failure", store.revoked)
	}
}

func TestSweepSkipsWhenSCIMDisabled(t *testing.T) {
	store := &fakeStore{candidates: []repository.UserSummary{{ID: 1, Email: "a@example.com"}}}
	sweeper := New(store, &fakeDir{enabled: false}, Config{}, quietLogger())
	// Run returns immediately rather than sweeping on a fallback directory,
	// where "missing" would be self-referential.
	sweeper.Run(context.Background())
	if len(store.revoked) != 0 {
		t.Fatalf("revoked %v with SCIM disabled", store.revoked)
	}
}
