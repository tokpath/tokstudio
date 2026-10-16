ALTER TABLE payments_verified_facts ADD COLUMN valuation_version BIGINT NOT NULL DEFAULT 0;
ALTER TABLE payments_verified_facts ADD COLUMN valuation_reason TEXT NOT NULL DEFAULT '';
CREATE TABLE payments_receipt_valuations(
 id UUID PRIMARY KEY,fact_id TEXT NOT NULL REFERENCES payments_verified_facts(id),operator_id TEXT NOT NULL,
 version BIGINT NOT NULL CHECK(version>0),business_key TEXT NOT NULL,fingerprint TEXT NOT NULL,
 paid_at TIMESTAMPTZ,payment_time_evidence TEXT NOT NULL,valuation JSONB NOT NULL,value JSONB NOT NULL,
 evidence_reference TEXT NOT NULL,verified_by TEXT NOT NULL,created_at TIMESTAMPTZ NOT NULL,
 UNIQUE(fact_id,version),UNIQUE(operator_id,business_key)
);
CREATE FUNCTION payments_immutable_receipt_valuation() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'receipt valuation versions are immutable';END$$;
CREATE TRIGGER payments_receipt_valuations_immutable BEFORE UPDATE OR DELETE ON payments_receipt_valuations FOR EACH ROW EXECUTE FUNCTION payments_immutable_receipt_valuation();
CREATE INDEX payments_pending_valuations ON payments_verified_facts(operator_id,state,id) WHERE kind='receipt';
