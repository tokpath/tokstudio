ALTER TABLE remediation_refund_legs ADD COLUMN route_version bigint NOT NULL DEFAULT 1 CHECK(route_version>0);
ALTER TABLE remediation_refund_destination_proposals ADD COLUMN route_version bigint NOT NULL DEFAULT 1 CHECK(route_version>0);
ALTER TABLE remediation_refund_destination_authorizations ADD COLUMN route_version bigint NOT NULL DEFAULT 1 CHECK(route_version>0);
ALTER TABLE remediation_refund_destination_authorizations ADD COLUMN settled_before_usd numeric(38,18) NOT NULL DEFAULT 0 CHECK(settled_before_usd>=0);
ALTER TABLE remediation_refund_destination_authorizations DROP CONSTRAINT remediation_refund_destination_authorizations_leg_id_key;
ALTER TABLE remediation_refund_destination_authorizations ADD UNIQUE(leg_id,route_version);
ALTER TABLE remediation_refund_destination_authorizations ADD UNIQUE(leg_id,business_key);
CREATE TABLE remediation_refund_execution_closures(
 id uuid PRIMARY KEY,leg_id uuid NOT NULL REFERENCES remediation_refund_legs(id),operator_id uuid NOT NULL,
 route_version bigint NOT NULL CHECK(route_version>0),execution_key text NOT NULL,execution_mode text NOT NULL,payment_refund_id text NOT NULL,
 business_key text NOT NULL,fingerprint text NOT NULL,reason text NOT NULL CHECK(reason IN('failed','terminated','settled_partial')),
 no_further_payment_reference text NOT NULL CHECK(no_further_payment_reference<>''),evidence text NOT NULL CHECK(evidence<>''),actor_id uuid NOT NULL,
 settled_usd numeric(38,18) NOT NULL CHECK(settled_usd>=0),remaining_usd numeric(38,18) NOT NULL CHECK(remaining_usd>0),created_at timestamptz NOT NULL,
 UNIQUE(leg_id,route_version),UNIQUE(leg_id,business_key)
);
CREATE TRIGGER remediation_refund_execution_closures_immutable BEFORE UPDATE OR DELETE ON remediation_refund_execution_closures FOR EACH ROW EXECUTE FUNCTION remediation_immutable();
CREATE TABLE remediation_manual_refund_failures(
 id uuid PRIMARY KEY,leg_id uuid NOT NULL REFERENCES remediation_refund_legs(id),operator_id uuid NOT NULL,route_version bigint NOT NULL,
 execution_key text NOT NULL,business_key text NOT NULL,fingerprint text NOT NULL,result text NOT NULL CHECK(result IN('failed','cancelled')),
 external_reference text NOT NULL CHECK(external_reference<>''),no_further_payment_reference text NOT NULL CHECK(no_further_payment_reference<>''),
 evidence text NOT NULL CHECK(evidence<>''),verified_at timestamptz NOT NULL,verified_by uuid NOT NULL,created_at timestamptz NOT NULL,
 UNIQUE(leg_id,business_key),UNIQUE(leg_id,route_version,execution_key)
);
CREATE TRIGGER remediation_manual_refund_failures_immutable BEFORE UPDATE OR DELETE ON remediation_manual_refund_failures FOR EACH ROW EXECUTE FUNCTION remediation_immutable();
