CREATE TABLE remediation_policies (
 operator_id uuid PRIMARY KEY REFERENCES organization_operators(id),
 second_review_above numeric(38,18) CHECK(second_review_above>=0), version bigint NOT NULL CHECK(version>0), updated_by uuid NOT NULL, updated_at timestamptz NOT NULL
);
CREATE TABLE remediation_cases (
 id uuid PRIMARY KEY, operator_id uuid NOT NULL REFERENCES organization_operators(id), customer_id uuid REFERENCES organization_customers(id),
 business_key text NOT NULL, fingerprint text NOT NULL, kind text NOT NULL CHECK(kind IN ('wallet','package_exception','unallocated')),
 order_id uuid, grant_id uuid, grant_hold_id uuid REFERENCES ledger_holds(id),
 amount_usd numeric(38,18) NOT NULL CHECK(amount_usd>0), credit_atg numeric(38,18) NOT NULL DEFAULT 0 CHECK(credit_atg>=0),
 internal_usd numeric(38,18) NOT NULL DEFAULT 0 CHECK(internal_usd>=0), commission_usd numeric(38,18) NOT NULL DEFAULT 0 CHECK(commission_usd>=0),
 state text NOT NULL CHECK(state IN ('pending_review','approved','paying','completed','rejected','cancelled','exception')),
 reason text NOT NULL, evidence text NOT NULL, requested_by text NOT NULL, required_reviews integer NOT NULL CHECK(required_reviews IN(1,2)),
 created_at timestamptz NOT NULL, completed_at timestamptz, UNIQUE(operator_id,business_key)
);
CREATE TABLE remediation_reviews (
 case_id uuid NOT NULL REFERENCES remediation_cases(id), reviewer_id uuid NOT NULL, decision text NOT NULL CHECK(decision IN('approve','reject')), evidence text NOT NULL,
 created_at timestamptz NOT NULL, PRIMARY KEY(case_id,reviewer_id)
);
CREATE TABLE remediation_refund_legs (
 id uuid PRIMARY KEY, case_id uuid NOT NULL REFERENCES remediation_cases(id), operator_id uuid NOT NULL REFERENCES organization_operators(id), receipt_id uuid NOT NULL REFERENCES commerce_receipts(id),
 amount_usd numeric(38,18) NOT NULL CHECK(amount_usd>0), hold_id uuid REFERENCES ledger_holds(id), source_account_id uuid REFERENCES ledger_accounts(id),
 state text NOT NULL CHECK(state IN('reserved','approved','executing','unknown','completed','cancelled','exception')),
 execution_key text NOT NULL DEFAULT '', execution_mode text NOT NULL DEFAULT '', execution_reference text NOT NULL DEFAULT '',
 payment_refund_id text NOT NULL DEFAULT '', completed_at timestamptz, created_at timestamptz NOT NULL,
 UNIQUE(case_id,receipt_id)
);
CREATE INDEX remediation_refund_receipts ON remediation_refund_legs(operator_id,receipt_id,state);
CREATE TABLE remediation_payment_observations (
 id uuid PRIMARY KEY, leg_id uuid NOT NULL REFERENCES remediation_refund_legs(id), business_key text NOT NULL, fingerprint text NOT NULL,
 actual_usd numeric(38,18) NOT NULL CHECK(actual_usd>0), currency text NOT NULL, amount text NOT NULL,
 fee_status text NOT NULL CHECK(fee_status IN('pending','confirmed')), fee_usd numeric(38,18) CHECK(fee_usd>=0),
 evidence text NOT NULL, occurred_at timestamptz NOT NULL, created_at timestamptz NOT NULL, UNIQUE(leg_id,business_key),
 CHECK((fee_status='pending' AND fee_usd IS NULL) OR (fee_status='confirmed' AND fee_usd IS NOT NULL))
);
CREATE TABLE remediation_events (
 id uuid PRIMARY KEY, operator_id uuid NOT NULL, subject_id uuid NOT NULL, action text NOT NULL, actor_id text NOT NULL, evidence text NOT NULL, data jsonb NOT NULL, created_at timestamptz NOT NULL
);
CREATE FUNCTION remediation_immutable() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'remediation evidence is immutable'; END $$;
CREATE TRIGGER remediation_reviews_immutable BEFORE UPDATE OR DELETE ON remediation_reviews FOR EACH ROW EXECUTE FUNCTION remediation_immutable();
CREATE TRIGGER remediation_events_immutable BEFORE UPDATE OR DELETE ON remediation_events FOR EACH ROW EXECUTE FUNCTION remediation_immutable();
CREATE TRIGGER remediation_payment_observations_immutable BEFORE UPDATE OR DELETE ON remediation_payment_observations FOR EACH ROW EXECUTE FUNCTION remediation_immutable();
CREATE TABLE remediation_wallet_links (leg_id uuid NOT NULL REFERENCES remediation_refund_legs(id), allocation_id uuid NOT NULL REFERENCES commerce_wallet_funding_allocations(id), PRIMARY KEY(leg_id,allocation_id));
CREATE TABLE remediation_chargebacks (
 id uuid PRIMARY KEY, operator_id uuid NOT NULL REFERENCES organization_operators(id), receipt_id uuid NOT NULL REFERENCES commerce_receipts(id),
 provider text NOT NULL, provider_case_id text NOT NULL, fingerprint text NOT NULL, amount_usd numeric(38,18) NOT NULL CHECK(amount_usd>0),
 recovered_usd numeric(38,18) NOT NULL DEFAULT 0 CHECK(recovered_usd>=0 AND recovered_usd<=amount_usd),
 restrictions_applied_at timestamptz,
 state text NOT NULL CHECK(state IN('disputed','invalid_payment','merchant_collection_risk','reversed')),
 reason text NOT NULL, evidence text NOT NULL, created_at timestamptz NOT NULL, resolved_at timestamptz, UNIQUE(operator_id,provider,provider_case_id)
);
CREATE TABLE remediation_refund_capacity (
 operator_id uuid NOT NULL,receipt_id uuid NOT NULL REFERENCES commerce_receipts(id),reference text NOT NULL,
 amount_usd numeric(38,18) NOT NULL CHECK(amount_usd>0),state text NOT NULL CHECK(state IN('reserved','completed','released')),
 created_at timestamptz NOT NULL,resolved_at timestamptz,PRIMARY KEY(operator_id,receipt_id,reference)
);
ALTER TABLE remediation_refund_legs ADD COLUMN source_operator_id uuid NOT NULL REFERENCES organization_operators(id);
CREATE TABLE remediation_chargeback_holds (
 id uuid PRIMARY KEY, chargeback_id uuid NOT NULL REFERENCES remediation_chargebacks(id), operator_id uuid NOT NULL,
 customer_id uuid, grant_id uuid, order_id uuid, wallet_reservation_id uuid, hold_id uuid NOT NULL REFERENCES ledger_holds(id), source_account_id uuid NOT NULL,
 amount numeric(38,18) NOT NULL CHECK(amount>0), kind text NOT NULL CHECK(kind IN('wallet','grant','unallocated','order_funds')),state text NOT NULL CHECK(state IN('held','released','cancelled')),created_at timestamptz NOT NULL,
 UNIQUE(chargeback_id,source_account_id)
);
CREATE TABLE remediation_chargeback_order_claims (
 chargeback_id uuid NOT NULL REFERENCES remediation_chargebacks(id),receipt_id uuid NOT NULL REFERENCES commerce_receipts(id),order_id uuid NOT NULL,
 amount_usd numeric(38,18) NOT NULL CHECK(amount_usd>0),state text NOT NULL CHECK(state IN('active','invalid','released')),
 PRIMARY KEY(chargeback_id,order_id)
);
CREATE TABLE remediation_chargeback_events (
 chargeback_id uuid NOT NULL REFERENCES remediation_chargebacks(id), source_event_id uuid NOT NULL, operator_id uuid NOT NULL,
 amount_usd numeric(38,18) NOT NULL CHECK(amount_usd>0),state text NOT NULL CHECK(state IN('disputed','observed','released','reversed')),PRIMARY KEY(chargeback_id,source_event_id)
);
CREATE TABLE remediation_chargeback_recoveries(id uuid PRIMARY KEY,chargeback_id uuid NOT NULL REFERENCES remediation_chargebacks(id),business_key text NOT NULL,amount_usd numeric(38,18) NOT NULL CHECK(amount_usd>0),evidence text NOT NULL,created_at timestamptz NOT NULL,UNIQUE(chargeback_id,business_key));
CREATE TRIGGER remediation_chargeback_recoveries_immutable BEFORE UPDATE OR DELETE ON remediation_chargeback_recoveries FOR EACH ROW EXECUTE FUNCTION remediation_immutable();
