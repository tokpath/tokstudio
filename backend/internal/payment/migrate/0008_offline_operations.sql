-- External references are optional evidence. System operations provide identity.
-- Nonempty values are real external transaction IDs, unique within the payee.
-- Free-form notes use receipt_note and never participate in uniqueness.
CREATE UNIQUE INDEX IF NOT EXISTS payment_offline_receipt_unique ON payment_orders(payee_channel_org_id, receipt_reference) WHERE receipt_reference <> '';
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS received_at TIMESTAMPTZ;
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS receipt_note TEXT NOT NULL DEFAULT '';
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS refunded_at TIMESTAMPTZ;
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS refund_recorded_by TEXT NOT NULL DEFAULT '';
CREATE TABLE IF NOT EXISTS payment_offline_operations (
  operation_id TEXT PRIMARY KEY,
  actor_user_id TEXT NOT NULL,
  payee_channel_org_id TEXT NOT NULL,
  payload_json JSONB NOT NULL,
  order_id TEXT NOT NULL REFERENCES payment_orders(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS payment_offline_operations_actor_idx
  ON payment_offline_operations(actor_user_id, payee_channel_org_id, created_at DESC);
