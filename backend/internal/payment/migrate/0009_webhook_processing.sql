-- A received callback and its local financial application are separate facts.
ALTER TABLE payment_events ADD COLUMN provider_trade_id TEXT NOT NULL DEFAULT '';
ALTER TABLE payment_events ADD COLUMN status TEXT NOT NULL DEFAULT '';
ALTER TABLE payment_events ADD COLUMN applied_at TIMESTAMPTZ;
ALTER TABLE payment_events ADD COLUMN processing_error TEXT NOT NULL DEFAULT '';
ALTER TABLE payment_orders ADD COLUMN refund_status TEXT NOT NULL DEFAULT '';
ALTER TABLE payment_orders ADD COLUMN refund_amount_minor BIGINT;
CREATE INDEX payment_events_unapplied_idx ON payment_events(processed_at) WHERE applied_at IS NULL;
