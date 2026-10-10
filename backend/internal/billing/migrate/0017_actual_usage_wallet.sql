-- Only admitted API requests may settle beyond their estimate. Cash available
-- can be negative; gift, commission and concurrent reservations never can.
ALTER TABLE billing_wallets DROP CONSTRAINT IF EXISTS billing_wallets_nonneg;
ALTER TABLE billing_wallets ADD CONSTRAINT billing_wallets_nonneg CHECK (
    reserved_minor >= 0 AND gift_minor >= 0 AND commission_available_minor >= 0
);
ALTER TABLE billing_authorizations ADD COLUMN entitlement_settled_minor BIGINT NOT NULL DEFAULT 0 CHECK (entitlement_settled_minor >= 0);
UPDATE billing_authorizations SET entitlement_settled_minor = LEAST(settled_minor, GREATEST(amount_minor-wallet_reserved_minor,0)) WHERE status IN ('settled','reversed');
