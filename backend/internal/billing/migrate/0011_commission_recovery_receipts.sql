ALTER TABLE billing_commission_recoveries
    ADD COLUMN recovered_minor BIGINT NOT NULL DEFAULT 0,
    ADD CONSTRAINT billing_commission_recoveries_recovered_valid
        CHECK (recovered_minor >= 0 AND recovered_minor <= amount_minor);
CREATE TABLE billing_commission_recovery_receipts (
    id TEXT PRIMARY KEY,
    recovery_id TEXT NOT NULL REFERENCES billing_commission_recoveries(id),
    amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),
    reference TEXT NOT NULL,
    note TEXT NOT NULL DEFAULT '',
    actor_user_id TEXT NOT NULL,
    idempotency_key TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (recovery_id, reference)
);
CREATE INDEX billing_commission_recovery_receipts_recovery_idx
    ON billing_commission_recovery_receipts(recovery_id, created_at);
