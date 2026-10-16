-- A refund destination is a separately authorized instruction. The original
-- chain sender can be an exchange or a contract and supplies no such consent.
CREATE TABLE remediation_refund_destination_proposals (
 id uuid PRIMARY KEY, leg_id uuid NOT NULL REFERENCES remediation_refund_legs(id),
 operator_id uuid NOT NULL, customer_id uuid NOT NULL REFERENCES organization_customers(id),
 business_key text NOT NULL, fingerprint text NOT NULL,
 network text NOT NULL, asset text NOT NULL, token_contract text NOT NULL, address text NOT NULL,
 authorization_reference text NOT NULL CHECK(authorization_reference<>''), created_at timestamptz NOT NULL,
 UNIQUE(leg_id,business_key)
);
CREATE INDEX remediation_destination_proposals_latest ON remediation_refund_destination_proposals(leg_id,created_at DESC,id DESC);
CREATE TABLE remediation_refund_destination_authorizations (
 id uuid PRIMARY KEY, leg_id uuid NOT NULL UNIQUE REFERENCES remediation_refund_legs(id),
 operator_id uuid NOT NULL, receipt_id uuid NOT NULL REFERENCES commerce_receipts(id),
 proposal_id uuid REFERENCES remediation_refund_destination_proposals(id),
 customer_id uuid REFERENCES organization_customers(id), amount_usd numeric(38,18) NOT NULL CHECK(amount_usd>0),
 business_key text NOT NULL, fingerprint text NOT NULL,
 network text NOT NULL, asset text NOT NULL, token_contract text NOT NULL, address text NOT NULL,
 payer_authorization_reference text NOT NULL CHECK(payer_authorization_reference<>''),
 verification_evidence text NOT NULL CHECK(verification_evidence<>''), verified_by uuid NOT NULL,
 created_at timestamptz NOT NULL
);
CREATE TRIGGER remediation_destination_proposals_immutable BEFORE UPDATE OR DELETE ON remediation_refund_destination_proposals FOR EACH ROW EXECUTE FUNCTION remediation_immutable();
CREATE TRIGGER remediation_destination_authorizations_immutable BEFORE UPDATE OR DELETE ON remediation_refund_destination_authorizations FOR EACH ROW EXECUTE FUNCTION remediation_immutable();
CREATE TABLE remediation_external_refund_observations (
 observation_id uuid PRIMARY KEY REFERENCES remediation_payment_observations(id),
 authorization_id uuid NOT NULL REFERENCES remediation_refund_destination_authorizations(id),
 network text NOT NULL, asset text NOT NULL, token_contract text NOT NULL, address text NOT NULL,
 transaction_hash text NOT NULL, log_index text NOT NULL, block_hash text NOT NULL,
 sender_address text NOT NULL, recipient_address text NOT NULL,
 confirmations text NOT NULL, verification_method text NOT NULL, verified_by text NOT NULL,
 created_at timestamptz NOT NULL
);
CREATE TRIGGER remediation_external_refund_observations_immutable BEFORE UPDATE OR DELETE ON remediation_external_refund_observations FOR EACH ROW EXECUTE FUNCTION remediation_immutable();
