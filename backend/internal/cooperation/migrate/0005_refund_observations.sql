ALTER TABLE cooperation_procurement_exit_refunds ADD COLUMN settled_usd NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK(settled_usd>=0 AND settled_usd<=amount_usd);
ALTER TABLE cooperation_procurement_exit_refunds ADD COLUMN actual_usd NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK(actual_usd>=0);
ALTER TABLE cooperation_procurement_exit_refunds ADD COLUMN excess_usd NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK(excess_usd>=0);
UPDATE cooperation_procurement_exit_refunds SET settled_usd=amount_usd,actual_usd=amount_usd WHERE state='completed';
CREATE TABLE cooperation_procurement_refund_observations(
 id UUID PRIMARY KEY,leg_id UUID NOT NULL REFERENCES cooperation_procurement_exit_refunds(id),fact_id TEXT NOT NULL UNIQUE,
 actual_usd NUMERIC(38,18) NOT NULL CHECK(actual_usd>=0),settled_usd NUMERIC(38,18) NOT NULL CHECK(settled_usd>=0),excess_usd NUMERIC(38,18) NOT NULL CHECK(excess_usd>=0),
 recipient_matches BOOLEAN NOT NULL,money JSONB NOT NULL,valuation JSONB NOT NULL,external_execution JSONB,occurred_at TIMESTAMPTZ,created_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE cooperation_procurement_refund_instructions(
 leg_id UUID NOT NULL REFERENCES cooperation_procurement_exit_refunds(id),execution_key TEXT NOT NULL,remaining_usd NUMERIC(38,18) NOT NULL CHECK(remaining_usd>0),
 evidence TEXT NOT NULL,authorized_by UUID NOT NULL,created_at TIMESTAMPTZ NOT NULL,PRIMARY KEY(leg_id,execution_key)
);
CREATE TRIGGER cooperation_refund_observations_immutable BEFORE UPDATE OR DELETE ON cooperation_procurement_refund_observations FOR EACH ROW EXECUTE FUNCTION ledger_immutable_fact();
CREATE TRIGGER cooperation_refund_instructions_immutable BEFORE UPDATE OR DELETE ON cooperation_procurement_refund_instructions FOR EACH ROW EXECUTE FUNCTION ledger_immutable_fact();
