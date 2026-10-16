ALTER TABLE ledger_grants ADD COLUMN original_operator_id uuid;
UPDATE ledger_grants SET original_operator_id=operator_id;
ALTER TABLE ledger_grants ALTER COLUMN original_operator_id SET NOT NULL;
CREATE TABLE ledger_customer_transfers (
 id uuid PRIMARY KEY, takeover_id uuid NOT NULL, from_operator_id uuid NOT NULL, to_operator_id uuid NOT NULL, customer_id uuid NOT NULL,
 fingerprint text NOT NULL, account_ids jsonb NOT NULL, hold_ids jsonb NOT NULL, actor_id text NOT NULL, evidence text NOT NULL, created_at timestamptz NOT NULL,
 UNIQUE(takeover_id,customer_id)
);
CREATE TRIGGER ledger_customer_transfers_immutable BEFORE UPDATE OR DELETE ON ledger_customer_transfers FOR EACH ROW EXECUTE FUNCTION ledger_immutable_fact();
