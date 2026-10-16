ALTER TABLE payments_verified_facts ADD COLUMN fact_recovery_after TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp();
CREATE INDEX payments_receipt_fact_recovery ON payments_verified_facts(fact_recovery_after,id) WHERE kind='receipt' AND COALESCE(receipt_id,'')='';
