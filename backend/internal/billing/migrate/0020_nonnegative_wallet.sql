-- Cash available cannot go below zero. Existing debt is cleared to zero
-- rather than carried forward; gift, commission and reservations stay non-negative.
UPDATE billing_wallets SET available_minor = 0 WHERE available_minor < 0;
UPDATE billing_wallets SET gift_minor = available_minor WHERE gift_minor > available_minor;
ALTER TABLE billing_wallets DROP CONSTRAINT IF EXISTS billing_wallets_nonneg;
ALTER TABLE billing_wallets ADD CONSTRAINT billing_wallets_nonneg CHECK (
    available_minor >= 0
    AND reserved_minor >= 0
    AND gift_minor >= 0
    AND commission_available_minor >= 0
    AND gift_minor <= available_minor
);
