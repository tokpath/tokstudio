ALTER TABLE cooperation_refund_destination_proposals ADD COLUMN version BIGINT NOT NULL DEFAULT 1 CHECK(version>0);
ALTER TABLE cooperation_refund_destination_authorizations DROP CONSTRAINT cooperation_refund_destination_authorizations_leg_id_key;
ALTER TABLE cooperation_refund_destination_authorizations ADD COLUMN version BIGINT NOT NULL DEFAULT 1 CHECK(version>0);
ALTER TABLE cooperation_refund_destination_authorizations ADD COLUMN execution_merchant_id TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX cooperation_refund_authorization_version ON cooperation_refund_destination_authorizations(leg_id,version);
ALTER TABLE cooperation_procurement_refund_instructions ADD COLUMN authorization_id UUID;
CREATE TABLE cooperation_refund_route_closures(
 id UUID PRIMARY KEY,leg_id UUID NOT NULL REFERENCES cooperation_procurement_exit_refunds(id),version BIGINT NOT NULL CHECK(version>0),
 remaining_usd NUMERIC(38,18) NOT NULL CHECK(remaining_usd>0),reason TEXT NOT NULL,evidence TEXT NOT NULL,
 payment_facts JSONB NOT NULL,closed_by UUID NOT NULL,created_at TIMESTAMPTZ NOT NULL,UNIQUE(leg_id,version)
);
CREATE TABLE cooperation_refund_manual_failures(
 id UUID PRIMARY KEY,leg_id UUID NOT NULL REFERENCES cooperation_procurement_exit_refunds(id),business_key TEXT NOT NULL,
 execution_key TEXT NOT NULL,result TEXT NOT NULL CHECK(result IN('failed','cancelled')),external_reference TEXT NOT NULL,
 no_further_payment_reference TEXT NOT NULL,evidence TEXT NOT NULL,verified_by UUID NOT NULL,verified_at TIMESTAMPTZ NOT NULL,
 created_at TIMESTAMPTZ NOT NULL,UNIQUE(leg_id,business_key),UNIQUE(leg_id,execution_key)
);
CREATE TRIGGER cooperation_refund_route_closures_immutable BEFORE UPDATE OR DELETE ON cooperation_refund_route_closures FOR EACH ROW EXECUTE FUNCTION ledger_immutable_fact();
CREATE TRIGGER cooperation_refund_manual_failures_immutable BEFORE UPDATE OR DELETE ON cooperation_refund_manual_failures FOR EACH ROW EXECUTE FUNCTION ledger_immutable_fact();
