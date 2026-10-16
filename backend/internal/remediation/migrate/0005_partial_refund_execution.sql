ALTER TABLE remediation_refund_capacity ADD COLUMN completed_usd numeric(38,18) NOT NULL DEFAULT 0 CHECK(completed_usd>=0 AND completed_usd<=amount_usd);
UPDATE remediation_refund_capacity SET completed_usd=amount_usd WHERE state='completed';
CREATE TABLE remediation_receipt_refund_observations(
 operator_id uuid NOT NULL,receipt_id uuid NOT NULL REFERENCES commerce_receipts(id),reference text NOT NULL,fact_id text NOT NULL,
 actual_usd numeric(38,18) NOT NULL CHECK(actual_usd>0),settled_usd numeric(38,18) NOT NULL CHECK(settled_usd>=0),excess_usd numeric(38,18) NOT NULL CHECK(excess_usd>=0),remaining_usd numeric(38,18) NOT NULL CHECK(remaining_usd>=0),created_at timestamptz NOT NULL,
 PRIMARY KEY(operator_id,receipt_id,fact_id),UNIQUE(operator_id,fact_id),FOREIGN KEY(operator_id,receipt_id,reference) REFERENCES remediation_refund_capacity(operator_id,receipt_id,reference),CHECK(settled_usd+excess_usd=actual_usd)
);
CREATE TRIGGER remediation_receipt_refund_observations_immutable BEFORE UPDATE OR DELETE ON remediation_receipt_refund_observations FOR EACH ROW EXECUTE FUNCTION remediation_immutable();
ALTER TABLE remediation_refund_legs ADD COLUMN settled_usd numeric(38,18) NOT NULL DEFAULT 0 CHECK(settled_usd>=0 AND settled_usd<=amount_usd);
ALTER TABLE remediation_refund_legs ADD COLUMN excess_usd numeric(38,18) NOT NULL DEFAULT 0 CHECK(excess_usd>=0);
ALTER TABLE remediation_refund_legs ADD COLUMN misdirected_usd numeric(38,18) NOT NULL DEFAULT 0 CHECK(misdirected_usd>=0);
UPDATE remediation_refund_legs SET settled_usd=amount_usd WHERE state='completed';
CREATE TABLE remediation_payment_fact_bindings(
 operator_id uuid NOT NULL,fact_id text NOT NULL,leg_id uuid NOT NULL REFERENCES remediation_refund_legs(id),observation_id uuid NOT NULL UNIQUE REFERENCES remediation_payment_observations(id),fingerprint text NOT NULL,
 PRIMARY KEY(operator_id,fact_id)
);
CREATE TRIGGER remediation_payment_fact_bindings_immutable BEFORE UPDATE OR DELETE ON remediation_payment_fact_bindings FOR EACH ROW EXECUTE FUNCTION remediation_immutable();
