-- Unpaid and paid-but-undelivered procurement orders no longer expire.
-- Their sales commitment keeps holding supplier inventory until payment
-- and delivery, or an explicit cancellation.
UPDATE cooperation_purchase_orders
SET expires_at = TIMESTAMPTZ '9999-01-01 00:00:00+00'
WHERE status IN ('awaiting_payment', 'paid');
