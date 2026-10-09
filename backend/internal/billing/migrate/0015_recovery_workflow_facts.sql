ALTER TABLE billing_commission_recovery_receipts
    ADD COLUMN occurred_at TIMESTAMPTZ,
    ADD COLUMN scope_id TEXT NOT NULL DEFAULT '';
UPDATE billing_commission_recovery_receipts SET scope_id='legacy:' || recovery_id;
ALTER TABLE billing_commission_recovery_receipts
    DROP CONSTRAINT billing_commission_recovery_receipts_recovery_id_reference_key,
    DROP CONSTRAINT billing_commission_recovery_receipts_idempotency_key_key;
CREATE UNIQUE INDEX billing_recovery_operation_scope_idx
    ON billing_commission_recovery_receipts(scope_id, actor_user_id, idempotency_key);
CREATE UNIQUE INDEX billing_recovery_reference_scope_idx
    ON billing_commission_recovery_receipts(scope_id, reference) WHERE reference <> '';
