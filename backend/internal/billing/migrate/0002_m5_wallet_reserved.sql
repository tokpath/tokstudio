-- 预授权里钱包实际冻结的部分，其余由套餐权益覆盖。
ALTER TABLE billing_authorizations
    ADD COLUMN IF NOT EXISTS wallet_reserved_minor BIGINT NOT NULL DEFAULT 0;

UPDATE billing_authorizations
SET wallet_reserved_minor = amount_minor
WHERE wallet_reserved_minor = 0 AND amount_minor > 0 AND status IN ('reserved', 'pending_reconciliation', 'settled');
