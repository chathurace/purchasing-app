-- Append-only list of approval decisions on a contract. The contract's own
-- status is the rolled-up state; multiple rows (even from the same approver)
-- are allowed by design ("one or more approvals").
CREATE TABLE contract_approvals (
    id          BIGSERIAL   PRIMARY KEY,
    contract_id BIGINT      NOT NULL REFERENCES contracts(id) ON DELETE CASCADE,
    approver_id BIGINT      NOT NULL REFERENCES users(id),
    decision    TEXT        NOT NULL,                      -- approve | reject
    comment     TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_contract_approvals_contract ON contract_approvals(contract_id);
