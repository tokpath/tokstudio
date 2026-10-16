CREATE TABLE remediation_refund_fee_confirmations (
 id uuid PRIMARY KEY, operator_id uuid NOT NULL REFERENCES organization_operators(id),
 observation_id uuid NOT NULL UNIQUE REFERENCES remediation_payment_observations(id),
 leg_id uuid NOT NULL REFERENCES remediation_refund_legs(id),
 business_key text NOT NULL, fingerprint text NOT NULL, fee_usd numeric(38,18) NOT NULL CHECK(fee_usd>=0),
 journal_id uuid REFERENCES ledger_journals(id), reference text NOT NULL, evidence text NOT NULL,
 actor_id text NOT NULL, occurred_at timestamptz NOT NULL, created_at timestamptz NOT NULL,
 UNIQUE(observation_id,business_key), CHECK((fee_usd=0 AND journal_id IS NULL) OR (fee_usd>0 AND journal_id IS NOT NULL))
);
CREATE TRIGGER remediation_refund_fees_immutable BEFORE UPDATE OR DELETE ON remediation_refund_fee_confirmations FOR EACH ROW EXECUTE FUNCTION remediation_immutable();
