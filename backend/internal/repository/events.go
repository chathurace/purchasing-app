package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/cs/purchasing-app/internal/model"
)

// ProcessEvent is one row of the append-only PR business-process log.
type ProcessEvent struct {
	ID                int64     `json:"id"`
	PurchaseRequestID int64     `json:"purchase_request_id"`
	Action            string    `json:"action"`
	Qualifier         string    `json:"qualifier"`
	ActorID           *int64    `json:"actor_id"`
	ActorEmail        string    `json:"actor_email"`
	CreatedAt         time.Time `json:"created_at"`
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
	CreatedAt  time.Time `json:"created_at"`
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

// nullableID maps a non-positive id to nil so it is stored as SQL NULL.
func nullableID(id int64) *int64 {
	if id <= 0 {
		return nil
	}
	return &id
}
