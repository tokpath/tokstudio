CREATE INDEX payments_intents_by_order ON payments_intents(operator_id,order_id);
CREATE INDEX payments_receipt_facts_by_order ON payments_verified_facts(operator_id,order_id) WHERE kind='receipt';
