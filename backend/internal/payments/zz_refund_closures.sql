ALTER TABLE payments_refund_intents ADD COLUMN query_lease_until TIMESTAMPTZ;
CREATE TABLE payments_refund_execution_closures(
 operator_id TEXT NOT NULL,merchant_id TEXT NOT NULL,execution_id TEXT NOT NULL,
 business_refund_id TEXT NOT NULL,closure_id TEXT NOT NULL,closed_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(merchant_id,execution_id),
 FOREIGN KEY(merchant_id,execution_id) REFERENCES payments_refund_intents(merchant_id,refund_id)
);
CREATE FUNCTION payments_immutable_refund_closure() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'refund execution closures are immutable';END$$;
CREATE TRIGGER payments_refund_execution_closures_immutable BEFORE UPDATE OR DELETE ON payments_refund_execution_closures FOR EACH ROW EXECUTE FUNCTION payments_immutable_refund_closure();
