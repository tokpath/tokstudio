-- Snapshot the original payee; never redirect historical refunds to another merchant.
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS payee_channel_org_id TEXT NOT NULL DEFAULT '';
UPDATE payment_orders SET payee_channel_org_id = channel_org_id WHERE payee_channel_org_id = '';
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS receipt_reference TEXT NOT NULL DEFAULT '';
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS recorded_by TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS payment_orders_payee_idx ON payment_orders(payee_channel_org_id, created_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS payment_offline_receipt_unique ON payment_orders(payee_channel_org_id, receipt_reference) WHERE receipt_reference <> '';
