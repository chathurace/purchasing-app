// Package offboard signs users out of the app once the identity server says
// they should no longer have access.
//
// Why it exists: the app's session is a cookie backed by a database row, and
// after that row is created the IdP is never consulted again — that is what lets
// a session outlive an ID token. The consequence is that an account **deleted or
// disabled at the IdP keeps working here** until someone revokes it by hand or
// the absolute cap lapses. OIDC back-channel logout is the push-based fix, but it
// needs a per-app callback URL registered at the IdP, which Asgardeo does not
// expose for single-page-application registrations.
//
// So this is the pull-based fix, and it reuses machinery already in the app: the
// SCIM directory the autocomplete pickers are built on already fetches the full
// user list every TTL. Comparing the users holding live sessions against that
// snapshot identifies exactly the accounts that should be out.
//
// It intentionally only **revokes sessions**; it never deactivates the app user.
// Revocation is self-correcting — if the IdP account comes back, the person signs
// in again and nothing needs undoing — whereas an automated in-app deactivation
// would need an automated reactivation to match, and would silently fight an
// admin who re-enabled someone by hand.
package offboard

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cs/purchasing-app/internal/directory"
	"github.com/cs/purchasing-app/internal/model"
	"github.com/cs/purchasing-app/internal/repository"
	"github.com/rs/zerolog"
)

// Reason says why an account is being signed out, and becomes the audit detail.
type Reason string

const (
	// ReasonMissing — the account is not in the IdP directory at all (deleted).
	ReasonMissing Reason = "not present in the identity server"
	// ReasonDisabled — present but flagged inactive at the IdP.
	ReasonDisabled Reason = "disabled in the identity server"
)

// Candidate is a user holding at least one live session.
type Candidate struct {
	UserID int64
	Email  string
}

// Target is a candidate the sweep has decided to sign out.
type Target struct {
	Candidate
	Reason Reason
}

// Guard rails against a misconfigured or half-broken directory. The failure mode
// this exists to prevent: pointed at the wrong tenant (or a SCIM response that
// parses to a handful of users), every session in the app looks "missing" and
// everyone gets signed out at once.
//
// Note the blast radius even then is limited — revocation does not block signing
// back in — but a silent mass logout is still unacceptable, so a run that would
// hit most of the user base refuses to act and says so loudly instead.
const (
	// MaxRevocationRatio — refuse when a single run would sign out more than this
	// share of candidates...
	MaxRevocationRatio = 0.5
	// ...unless the numbers are small enough for that ratio to be meaningless
	// (2 of 3 users legitimately leaving on the same day is ordinary).
	MinRevocationsForRatioCheck = 5
)

// Decide returns the candidates that should be signed out, comparing them
// against an IdP snapshot. It is pure — all the safety rules live here so they
// can be tested without a database or a SCIM server.
//
// An error means "do nothing this run": the snapshot is not trustworthy enough to
// act on. Callers log it and wait for the next tick.
func Decide(candidates []Candidate, snapshot []directory.User) ([]Target, error) {
	// An empty snapshot cannot distinguish "everyone was deleted" from "the fetch
	// returned nothing useful". Never act on it.
	if len(snapshot) == 0 {
		return nil, fmt.Errorf("empty directory snapshot")
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	type entry struct{ active bool }
	idp := make(map[string]entry, len(snapshot))
	for _, u := range snapshot {
		email := normalizeEmail(u.Email)
		if email == "" {
			continue
		}
		// Duplicate emails in a directory are possible (multiple user stores).
		// Treat the identity as active if ANY matching entry is active, so a
		// stale duplicate cannot sign out a working account.
		if prev, ok := idp[email]; ok && prev.active {
			continue
		}
		idp[email] = entry{active: u.Active}
	}
	if len(idp) == 0 {
		return nil, fmt.Errorf("directory snapshot has %d entries but no usable emails", len(snapshot))
	}

	var targets []Target
	for _, c := range candidates {
		email := normalizeEmail(c.Email)
		if email == "" {
			continue // nothing to match on; leave it alone
		}
		e, found := idp[email]
		switch {
		case !found:
			targets = append(targets, Target{Candidate: c, Reason: ReasonMissing})
		case !e.active:
			targets = append(targets, Target{Candidate: c, Reason: ReasonDisabled})
		}
	}

	if len(targets) >= MinRevocationsForRatioCheck &&
		float64(len(targets)) > MaxRevocationRatio*float64(len(candidates)) {
		return nil, fmt.Errorf(
			"refusing to sign out %d of %d signed-in users in one run — the directory snapshot (%d entries) looks wrong",
			len(targets), len(candidates), len(snapshot))
	}
	return targets, nil
}

func normalizeEmail(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// --- the wiring around Decide -------------------------------------------------

// sessionStore is the persistence subset the sweeper needs, kept narrow so tests
// can stub it.
type sessionStore interface {
	UsersWithLiveSessions(ctx context.Context) ([]repository.UserSummary, error)
	RevokeAllUserSessions(ctx context.Context, userID int64) (int64, error)
	AddAuditEvent(ctx context.Context, action, qualifier, entityType string, entityID *int64, detail string, actorID int64, actorEmail string) error
}

// idpDirectory is the directory subset the sweeper needs.
type idpDirectory interface {
	Enabled() bool
	List(ctx context.Context, forceRefresh bool) ([]directory.User, error)
}

// Config controls the sweep.
type Config struct {
	// Interval between runs. Defaults to 10 minutes; there is no point running
	// much more often than the directory cache TTL, since that is how fresh the
	// snapshot can be.
	Interval time.Duration
	// DryRun logs what would be signed out and revokes nothing — the safe way to
	// watch a new deployment for a cycle before letting it act.
	DryRun bool
}

// Sweeper periodically signs out users the IdP no longer vouches for.
type Sweeper struct {
	store sessionStore
	dir   idpDirectory
	cfg   Config
	log   zerolog.Logger
}

func New(store sessionStore, dir idpDirectory, cfg Config, log zerolog.Logger) *Sweeper {
	if cfg.Interval <= 0 {
		cfg.Interval = 10 * time.Minute
	}
	return &Sweeper{store: store, dir: dir, cfg: cfg, log: log}
}

// Run sweeps on the configured interval until ctx is cancelled. It does NOT
// sweep immediately at startup: a boot-time SCIM failure is common (cold caches,
// slow networks) and the first thing this does when it cannot read the directory
// should not be to look like the feature is broken.
func (s *Sweeper) Run(ctx context.Context) {
	if !s.dir.Enabled() {
		// Without SCIM the directory falls back to the app's own users, so
		// "missing from the directory" would be self-referential and always false.
		s.log.Warn().Msg("idp offboarding sweep disabled: SCIM is not configured")
		return
	}
	s.log.Info().
		Dur("interval", s.cfg.Interval).
		Bool("dry_run", s.cfg.DryRun).
		Msg("idp offboarding sweep started")

	ticker := time.NewTicker(s.cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.Sweep(ctx)
		}
	}
}

// Sweep runs one pass. Exported so it can be driven directly by a test.
func (s *Sweeper) Sweep(ctx context.Context) {
	candidates, err := s.store.UsersWithLiveSessions(ctx)
	if err != nil {
		s.log.Error().Err(err).Msg("idp offboarding sweep: list signed-in users")
		return
	}
	if len(candidates) == 0 {
		return
	}

	// forceRefresh=false: the sweep rides the same cache the pickers use, so it
	// costs no extra SCIM calls. A stale snapshot is safe — it can hide a *new*
	// deletion (caught next run) but can never invent one.
	snapshot, err := s.dir.List(ctx, false)
	if err != nil {
		s.log.Error().Err(err).Msg("idp offboarding sweep: read directory")
		return
	}

	cs := make([]Candidate, 0, len(candidates))
	for _, u := range candidates {
		cs = append(cs, Candidate{UserID: u.ID, Email: u.Email})
	}

	targets, err := Decide(cs, snapshot)
	if err != nil {
		s.log.Error().Err(err).
			Int("signed_in_users", len(cs)).
			Int("directory_entries", len(snapshot)).
			Msg("idp offboarding sweep: refusing to act")
		return
	}
	if len(targets) == 0 {
		s.log.Debug().Int("signed_in_users", len(cs)).Msg("idp offboarding sweep: nothing to do")
		return
	}

	for _, t := range targets {
		if s.cfg.DryRun {
			s.log.Warn().
				Int64("user_id", t.UserID).
				Str("user", t.Email).
				Str("reason", string(t.Reason)).
				Msg("idp offboarding sweep (dry run): would sign out")
			continue
		}
		n, err := s.store.RevokeAllUserSessions(ctx, t.UserID)
		if err != nil {
			s.log.Error().Err(err).Int64("user_id", t.UserID).Msg("idp offboarding sweep: revoke")
			continue
		}
		if n == 0 {
			continue // raced with a logout; nothing to report
		}
		s.log.Warn().
			Int64("user_id", t.UserID).
			Str("user", t.Email).
			Str("reason", string(t.Reason)).
			Int64("revoked", n).
			Msg("signed out: identity server no longer grants access")

		// Audited with a NULL actor — the trigger is the identity server, not a
		// person. Best-effort: the revocation has already happened.
		userID := t.UserID
		if err := s.store.AddAuditEvent(ctx, model.AuditRevokeUserSessions, model.QualifierIdPOffboard,
			model.EntityUser, &userID, string(t.Reason), 0, ""); err != nil {
			s.log.Error().Err(err).Int64("user_id", userID).Msg("idp offboarding sweep: audit")
		}
	}
}
