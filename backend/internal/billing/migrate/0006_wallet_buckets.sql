-- 赠送积分与佣金钱包：API 只花 available（gift 是其中标签）；commission 不能抵 API。
ALTER TABLE billing_wallets
    ADD COLUMN IF NOT EXISTS gift_minor BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS commission_available_minor BIGINT NOT NULL DEFAULT 0;

ALTER TABLE billing_wallets DROP CONSTRAINT IF EXISTS billing_wallets_nonneg;
ALTER TABLE billing_wallets
    ADD CONSTRAINT billing_wallets_nonneg CHECK (
        available_minor >= 0
        AND reserved_minor >= 0
        AND gift_minor >= 0
        AND commission_available_minor >= 0
        AND gift_minor <= available_minor
    );

ALTER TABLE billing_authorizations
    ADD COLUMN IF NOT EXISTS gift_reserved_minor BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS gift_settled_minor BIGINT NOT NULL DEFAULT 0;
