-- Audit events: an append-only log of non-business-process mutations — master-data,
-- admin and configuration changes that are NOT tied to a purchase-request lifecycle
-- (add/edit a vendor, create/edit/delete a cost center or config option, create a
-- user, grant/revoke a role, activate/deactivate a user, connect storage, set the
-- storage folder). PR-lifecycle actions go to process_events instead.
--
-- Like process_events, the `action` set is FIXED in Go (model.AuditAction*) and
-- validated on write. `qualifier` carries a secondary dimension where relevant (the
-- role name for grant_role/revoke_role; active|inactive for set_user_active).
-- `entity_type`/`entity_id` identify the mutated record (entity_id is nullable —
-- storage has no id); `detail` holds optional human context such as the entity name.
-- The actor is stored as a FK plus an immutable email snapshot. Append-only.

CREATE TABLE audit_events (
    id          BIGSERIAL   PRIMARY KEY,
    action      TEXT        NOT NULL,                         -- fixed constant (model.AuditAction*)
    qualifier   TEXT        NOT NULL DEFAULT '',              -- role name for grant/revoke; active|inactive; ''
    entity_type TEXT        NOT NULL,                         -- vendor | cost_center | config_option | user | storage
    entity_id   BIGINT,                                       -- nullable (storage has no id)
    detail      TEXT        NOT NULL DEFAULT '',              -- human context, e.g. the entity name
    actor_id    BIGINT      REFERENCES users(id) ON DELETE SET NULL,
    actor_email TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_audit_events_entity ON audit_events(entity_type, entity_id);
CREATE INDEX idx_audit_events_action ON audit_events(action, created_at);
