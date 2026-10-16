CREATE TABLE payments_refund_fact_bindings(
 fact_id TEXT PRIMARY KEY,operator_id TEXT NOT NULL,merchant_id TEXT NOT NULL,business_refund_id TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE FUNCTION payments_immutable_refund_binding() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'refund facts have immutable obligation ownership';END$$;
CREATE TRIGGER payments_refund_binding_immutable BEFORE UPDATE OR DELETE ON payments_refund_fact_bindings FOR EACH ROW EXECUTE FUNCTION payments_immutable_refund_binding();
