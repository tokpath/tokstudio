CREATE INDEX ledger_grants_recent ON ledger_grants(operator_id, customer_id, created_at DESC, id DESC);
