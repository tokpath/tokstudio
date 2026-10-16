ALTER TABLE usage_requests ADD COLUMN corrected_atg numeric(38,18) NOT NULL DEFAULT 0 CHECK(corrected_atg>=0 AND corrected_atg<=actual_atg);
ALTER TABLE usage_requests ADD COLUMN corrected_charged_atg numeric(38,18) NOT NULL DEFAULT 0 CHECK(corrected_charged_atg>=0 AND corrected_charged_atg<=charged_atg);
CREATE TABLE usage_charge_corrections (
 id uuid PRIMARY KEY, request_id uuid NOT NULL REFERENCES usage_requests(id), operator_id uuid NOT NULL, business_key text NOT NULL,
 fingerprint text NOT NULL, amount_atg numeric(38,18) NOT NULL CHECK(amount_atg>0), returned_atg numeric(38,18) NOT NULL CHECK(returned_atg>=0),
 paid_usd numeric(38,18) NOT NULL CHECK(paid_usd>=0), reversals jsonb NOT NULL, evidence text NOT NULL, actor_id text NOT NULL, created_at timestamptz NOT NULL,
 UNIQUE(request_id,business_key)
);
CREATE TRIGGER usage_charge_corrections_immutable BEFORE UPDATE OR DELETE ON usage_charge_corrections FOR EACH ROW EXECUTE FUNCTION ledger_immutable_fact();
