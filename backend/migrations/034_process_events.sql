-- Process events: an append-only log of the significant business-process actions
-- performed on a purchase request, for BPMN-style process analysis. Each row is
-- one human task (submit PR, approve PR (legal), add quotation, sign contract, ...).
-- The `action` set is FIXED — it only changes when the code (the process) changes;
-- the canonical list lives in Go as model.ProcessAction* constants, and the
-- repository rejects any action outside that set. The decision direction of a task
-- (approve/reject/revert, sign/unsign, target invoice status, contract source) is a
-- `qualifier`, not a separate action. Only human actions are recorded — the PR
-- status auto-advances (under_review, vendor_selected, ...) are a side effect and
-- are derivable from the action sequence, so no "system" rows are written.
--
-- Append-only: never updated or deleted (rows cascade only if the PR itself is
-- deleted). The actor is stored both as a FK (actor_id) and as an immutable email
-- snapshot (actor_email) so analysis survives user renames/deletion and needs no join.

CREATE TABLE process_events (
    id                  BIGSERIAL   PRIMARY KEY,
    purchase_request_id BIGINT      NOT NULL REFERENCES purchase_requests(id) ON DELETE CASCADE,
    action              TEXT        NOT NULL,                 -- fixed task constant (model.ProcessAction*)
    qualifier           TEXT        NOT NULL DEFAULT '',      -- approve|reject|revert|sign|unsign|received|approved|paid|quotation|recommendation|''
    actor_id            BIGINT      REFERENCES users(id) ON DELETE SET NULL,
    actor_email         TEXT        NOT NULL DEFAULT '',      -- snapshot of the actor at event time
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()    -- "date"/"time" are derived: created_at::date, created_at::time
);
CREATE INDEX idx_process_events_pr     ON process_events(purchase_request_id, created_at);
CREATE INDEX idx_process_events_action ON process_events(action);
