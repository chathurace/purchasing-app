package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cs/purchasing-app/internal/model"
)

// ProcessEvent is one row of the append-only PR business-process log.
type ProcessEvent struct {
	ID                int64  `json:"id"`
	PurchaseRequestID int64  `json:"purchase_request_id"`
	Action            string `json:"action"`
	Qualifier         string `json:"qualifier"`
	ActorID           *int64 `json:"actor_id"`
	ActorEmail        string `json:"actor_email"`
	// ActorName is the actor's current display name, joined from users at read
	// time (empty when the actor row is gone or the read did not join it). The
	// immutable audit key is ActorEmail; ActorName is a display convenience.
	ActorName string    `json:"actor_name"`
	CreatedAt time.Time `json:"created_at"`
}

// AuditEvent is one row of the append-only non-process (master-data/admin) log.
type AuditEvent struct {
	ID         int64     `json:"id"`
	Action     string    `json:"action"`
	Qualifier  string    `json:"qualifier"`
	EntityType string    `json:"entity_type"`
	EntityID   *int64    `json:"entity_id"`
	Detail     string    `json:"detail"`
	ActorID    *int64    `json:"actor_id"`
	ActorEmail string    `json:"actor_email"`
	ActorName  string    `json:"actor_name"`
	CreatedAt  time.Time `json:"created_at"`
}

// EventFilter narrows an events read (both logs share it — PRID applies only to
// process events). Every field is optional; a zero value means "no filter".
type EventFilter struct {
	// ActorTerm is a case-insensitive substring matched against the actor's email
	// snapshot and current name.
	ActorTerm string
	// Action is an exact action match (e.g. "submit_pr", "grant_role").
	Action string
	// From / To are inclusive YYYY-MM-DD date bounds on created_at (server TZ).
	From string
	To   string
	// PRID matches process_events.purchase_request_id when non-nil (ignored for
	// audit events, which have no PR link).
	PRID *int64
	// Limit caps the rows returned newest-first; <=0 uses defaultEventLimit and a
	// value above maxEventLimit is clamped.
	Limit int
}

const (
	defaultEventLimit = 500
	maxEventLimit     = 2000
)

func (f EventFilter) limit() int {
	switch {
	case f.Limit <= 0:
		return defaultEventLimit
	case f.Limit > maxEventLimit:
		return maxEventLimit
	default:
		return f.Limit
	}
}

// AddProcessEvent appends a business-process event for a purchase request. The
// action must be one of the fixed model.ValidProcessActions — an unknown action
// is rejected (guarding the "fixed set" guarantee) rather than silently stored.
// created_at is DB-assigned. actorID <= 0 is stored as NULL.
func (r *Repository) AddProcessEvent(ctx context.Context, prID int64, action, qualifier string, actorID int64, actorEmail string) error {
	if !model.ValidProcessActions[action] {
		return fmt.Errorf("unknown process action %q", action)
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO process_events (purchase_request_id, action, qualifier, actor_id, actor_email)
		VALUES ($1, $2, $3, $4, $5)`,
		prID, action, qualifier, nullableID(actorID), actorEmail)
	return err
}

// AddAuditEvent appends a non-process (master-data/admin) event. The action must
// be one of the fixed model.ValidAuditActions. entityID nil is stored as NULL
// (e.g. storage, which has no id); actorID <= 0 is stored as NULL.
func (r *Repository) AddAuditEvent(ctx context.Context, action, qualifier, entityType string, entityID *int64, detail string, actorID int64, actorEmail string) error {
	if !model.ValidAuditActions[action] {
		return fmt.Errorf("unknown audit action %q", action)
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO audit_events (action, qualifier, entity_type, entity_id, detail, actor_id, actor_email)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		action, qualifier, entityType, entityID, detail, nullableID(actorID), actorEmail)
	return err
}

// ListProcessEvents returns a purchase request's process events oldest-first —
// the BPMN-style task timeline. No API exposes this yet (v1 is write-only); it
// backs verification and future analysis reads.
func (r *Repository) ListProcessEvents(ctx context.Context, prID int64) ([]ProcessEvent, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, purchase_request_id, action, qualifier, actor_id, actor_email, created_at
		FROM process_events
		WHERE purchase_request_id = $1
		ORDER BY created_at, id`, prID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ProcessEvent
	for rows.Next() {
		var e ProcessEvent
		if err := rows.Scan(&e.ID, &e.PurchaseRequestID, &e.Action, &e.Qualifier, &e.ActorID, &e.ActorEmail, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// eventWheres builds the shared WHERE fragments (actor/action/date) for a
// filtered events read, appending bind args to args and returning the fragments.
// It assumes the events table is aliased "e" and its actor's current name is
// available as u.name via a LEFT JOIN users u.
func eventWheres(f EventFilter, args *[]any) []string {
	var wheres []string
	if term := strings.TrimSpace(f.ActorTerm); term != "" {
		*args = append(*args, "%"+strings.ToLower(term)+"%")
		n := len(*args)
		wheres = append(wheres, fmt.Sprintf(`(LOWER(e.actor_email) LIKE $%d OR LOWER(COALESCE(u.name, '')) LIKE $%d)`, n, n))
	}
	if f.Action != "" {
		*args = append(*args, f.Action)
		wheres = append(wheres, fmt.Sprintf(`e.action = $%d`, len(*args)))
	}
	if f.From != "" {
		*args = append(*args, f.From)
		wheres = append(wheres, fmt.Sprintf(`e.created_at::date >= $%d::date`, len(*args)))
	}
	if f.To != "" {
		*args = append(*args, f.To)
		wheres = append(wheres, fmt.Sprintf(`e.created_at::date <= $%d::date`, len(*args)))
	}
	return wheres
}

// FilterProcessEvents returns process events newest-first across all PRs, narrowed
// by the filter, for the admin events view. Distinct from ListProcessEvents, which
// returns one PR's timeline oldest-first.
func (r *Repository) FilterProcessEvents(ctx context.Context, f EventFilter) ([]ProcessEvent, error) {
	args := []any{}
	wheres := eventWheres(f, &args)
	if f.PRID != nil {
		args = append(args, *f.PRID)
		wheres = append(wheres, fmt.Sprintf(`e.purchase_request_id = $%d`, len(args)))
	}
	query := `
		SELECT e.id, e.purchase_request_id, e.action, e.qualifier, e.actor_id,
		       e.actor_email, COALESCE(u.name, ''), e.created_at
		FROM process_events e
		LEFT JOIN users u ON u.id = e.actor_id`
	if len(wheres) > 0 {
		query += ` WHERE ` + strings.Join(wheres, ` AND `)
	}
	args = append(args, f.limit())
	query += fmt.Sprintf(` ORDER BY e.created_at DESC, e.id DESC LIMIT $%d`, len(args))

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ProcessEvent
	for rows.Next() {
		var e ProcessEvent
		if err := rows.Scan(&e.ID, &e.PurchaseRequestID, &e.Action, &e.Qualifier,
			&e.ActorID, &e.ActorEmail, &e.ActorName, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// FilterAuditEvents returns audit (master-data/admin) events newest-first, narrowed
// by the filter, for the admin events view. PRID is ignored (audit events have no
// PR link).
func (r *Repository) FilterAuditEvents(ctx context.Context, f EventFilter) ([]AuditEvent, error) {
	args := []any{}
	wheres := eventWheres(f, &args)
	query := `
		SELECT e.id, e.action, e.qualifier, e.entity_type, e.entity_id, e.detail,
		       e.actor_id, e.actor_email, COALESCE(u.name, ''), e.created_at
		FROM audit_events e
		LEFT JOIN users u ON u.id = e.actor_id`
	if len(wheres) > 0 {
		query += ` WHERE ` + strings.Join(wheres, ` AND `)
	}
	args = append(args, f.limit())
	query += fmt.Sprintf(` ORDER BY e.created_at DESC, e.id DESC LIMIT $%d`, len(args))

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditEvent
	for rows.Next() {
		var e AuditEvent
		if err := rows.Scan(&e.ID, &e.Action, &e.Qualifier, &e.EntityType, &e.EntityID,
			&e.Detail, &e.ActorID, &e.ActorEmail, &e.ActorName, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// nullableID maps a non-positive id to nil so it is stored as SQL NULL.
func nullableID(id int64) *int64 {
	if id <= 0 {
		return nil
	}
	return &id
}
