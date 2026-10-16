ALTER TABLE payments_refund_intents ADD COLUMN business_refund_id TEXT NOT NULL DEFAULT '';
UPDATE payments_refund_intents SET business_refund_id=refund_id;
CREATE INDEX payments_refund_business ON payments_refund_intents(operator_id,merchant_id,business_refund_id,created_at);
