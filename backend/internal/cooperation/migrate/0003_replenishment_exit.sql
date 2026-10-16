ALTER TABLE cooperation_procurement_lots ADD COLUMN paid_removed NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK(paid_removed>=0);
ALTER TABLE cooperation_procurement_lots ADD COLUMN gift_removed NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK(gift_removed>=0);
-- Replace only the source-conservation check; historical consumption stays intact.
DO $$ DECLARE c record; BEGIN FOR c IN SELECT conname FROM pg_constraint WHERE conrelid='cooperation_procurement_lots'::regclass AND contype='c' AND pg_get_constraintdef(oid) LIKE '%remaining%paid_atg%paid_consumed%' LOOP EXECUTE format('ALTER TABLE cooperation_procurement_lots DROP CONSTRAINT %I',c.conname); END LOOP; END $$;
ALTER TABLE cooperation_procurement_lots ADD CHECK(remaining=paid_atg+gift_atg-paid_consumed-gift_consumed-paid_removed-gift_removed);
ALTER TABLE cooperation_procurement_lots ADD CHECK(paid_consumed+paid_removed<=paid_atg AND gift_consumed+gift_removed<=gift_atg);
CREATE TABLE cooperation_procurement_removals(
 id UUID PRIMARY KEY,buyer_id UUID NOT NULL,business_key TEXT NOT NULL,fingerprint TEXT NOT NULL,
 mode TEXT NOT NULL,quantity NUMERIC(38,18) NOT NULL CHECK(quantity>0),book_cost_usd NUMERIC(38,18) NOT NULL CHECK(book_cost_usd>=0),
 cost_remainder_numerator TEXT NOT NULL,cost_remainder_denominator TEXT NOT NULL,
 sources JSONB NOT NULL,journal_id UUID NOT NULL,actor_id TEXT NOT NULL,reason TEXT NOT NULL,evidence TEXT NOT NULL,created_at TIMESTAMPTZ NOT NULL,
 UNIQUE(buyer_id,business_key)
);
CREATE TRIGGER cooperation_removals_immutable BEFORE UPDATE OR DELETE ON cooperation_procurement_removals FOR EACH ROW EXECUTE FUNCTION ledger_immutable_fact();
CREATE TABLE cooperation_procurement_exits(
 id UUID PRIMARY KEY,buyer_id UUID NOT NULL,supplier_id UUID NOT NULL,business_key TEXT NOT NULL,fingerprint TEXT NOT NULL,
 mode TEXT NOT NULL,removal_id UUID NOT NULL UNIQUE REFERENCES cooperation_procurement_removals(id),
 contract_usd NUMERIC(38,18) NOT NULL CHECK(contract_usd>=0),book_cost_usd NUMERIC(38,18) NOT NULL,
 cost_difference_usd NUMERIC(38,18) NOT NULL,contract_remainder_numerator TEXT NOT NULL,contract_remainder_denominator TEXT NOT NULL,
 state TEXT NOT NULL,evidence TEXT NOT NULL,created_by UUID NOT NULL,created_at TIMESTAMPTZ NOT NULL,
 UNIQUE(buyer_id,business_key)
);
CREATE TABLE cooperation_replenishment_authorizations(
 id UUID PRIMARY KEY,buyer_id UUID NOT NULL UNIQUE,supplier_id UUID NOT NULL,quote_id UUID NOT NULL,
 trigger_available_atg NUMERIC(38,18) NOT NULL CHECK(trigger_available_atg>=0),single_limit_usd NUMERIC(38,18) NOT NULL CHECK(single_limit_usd>0),
 period_limit_usd NUMERIC(38,18) NOT NULL CHECK(period_limit_usd>0),period TEXT NOT NULL CHECK(period IN('month','quarter','year')),
 mode TEXT NOT NULL CHECK(mode IN('automatic','offline')),merchant_id UUID,payment_ciphertext TEXT NOT NULL,
 consent_reference TEXT NOT NULL,enabled BOOLEAN NOT NULL,version BIGINT NOT NULL,authorized_by UUID NOT NULL,updated_at TIMESTAMPTZ NOT NULL,last_checked_at TIMESTAMPTZ NOT NULL DEFAULT '1970-01-01'
);
CREATE TABLE cooperation_replenishment_attempts(
 id UUID PRIMARY KEY,buyer_id UUID NOT NULL,authorization_id UUID NOT NULL REFERENCES cooperation_replenishment_authorizations(id),authorization_version BIGINT NOT NULL,
 purchase_order_id UUID NOT NULL UNIQUE REFERENCES cooperation_purchase_orders(id),snapshot_ciphertext TEXT NOT NULL,
 period_start TIMESTAMPTZ NOT NULL,period_end TIMESTAMPTZ NOT NULL,amount_usd NUMERIC(38,18) NOT NULL,
 state TEXT NOT NULL,provider_id TEXT NOT NULL DEFAULT '',error_code TEXT NOT NULL DEFAULT '',lease_until TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL,updated_at TIMESTAMPTZ NOT NULL
);
CREATE UNIQUE INDEX cooperation_one_replenishment_pending ON cooperation_replenishment_attempts(buyer_id) WHERE state NOT IN('completed','cancelled_unpaid','failed_unpaid');
CREATE TABLE cooperation_procurement_reversals(
 id UUID PRIMARY KEY,buyer_id UUID NOT NULL,request_id TEXT NOT NULL,business_key TEXT NOT NULL,fingerprint TEXT NOT NULL,
 quantity NUMERIC(38,18) NOT NULL CHECK(quantity>0),cost_usd NUMERIC(38,18) NOT NULL CHECK(cost_usd>=0),
 sources JSONB NOT NULL,journal_id UUID NOT NULL,actor_id TEXT NOT NULL,evidence TEXT NOT NULL,created_at TIMESTAMPTZ NOT NULL,
 UNIQUE(buyer_id,business_key)
);
CREATE TRIGGER cooperation_reversals_immutable BEFORE UPDATE OR DELETE ON cooperation_procurement_reversals FOR EACH ROW EXECUTE FUNCTION ledger_immutable_fact();
ALTER TABLE cooperation_procurement_exits ADD COLUMN refunded_usd NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK(refunded_usd>=0 AND refunded_usd<=contract_usd);
CREATE TABLE cooperation_procurement_exit_refunds(
 id UUID PRIMARY KEY,exit_id UUID NOT NULL REFERENCES cooperation_procurement_exits(id),buyer_id UUID NOT NULL,supplier_id UUID NOT NULL,
 business_key TEXT NOT NULL,fingerprint TEXT NOT NULL,receipt_id UUID NOT NULL,merchant_id UUID NOT NULL,origin_order_id TEXT NOT NULL,
 amount_usd NUMERIC(38,18) NOT NULL CHECK(amount_usd>0),hold_id UUID NOT NULL,
 state TEXT NOT NULL,verified_fact_id TEXT,execution_valuation JSONB,execution_mode TEXT NOT NULL DEFAULT '',execution_key TEXT NOT NULL DEFAULT '',execution_evidence TEXT NOT NULL DEFAULT '',created_by UUID NOT NULL,created_at TIMESTAMPTZ NOT NULL,last_checked_at TIMESTAMPTZ NOT NULL DEFAULT '1970-01-01',
 UNIQUE(exit_id,business_key)
);
CREATE TABLE cooperation_supply_failure_approvals(
 id UUID PRIMARY KEY,buyer_id UUID NOT NULL,supplier_id UUID NOT NULL,evidence TEXT NOT NULL,approved_by UUID NOT NULL,approved_at TIMESTAMPTZ NOT NULL
);
CREATE TRIGGER cooperation_supply_failure_immutable BEFORE UPDATE OR DELETE ON cooperation_supply_failure_approvals FOR EACH ROW EXECUTE FUNCTION ledger_immutable_fact();
ALTER TABLE cooperation_procurement_exit_refunds ADD COLUMN fee_status TEXT NOT NULL DEFAULT 'pending' CHECK(fee_status IN('pending','confirmed'));
ALTER TABLE cooperation_procurement_exit_refunds ADD COLUMN fee_usd NUMERIC(38,18) CHECK(fee_usd>=0);
ALTER TABLE cooperation_procurement_exit_refunds ADD CHECK((fee_status='pending' AND fee_usd IS NULL) OR (fee_status='confirmed' AND fee_usd IS NOT NULL));
CREATE TABLE cooperation_procurement_refund_accounting(
 refund_id UUID PRIMARY KEY REFERENCES cooperation_procurement_exit_refunds(id),business_key TEXT NOT NULL,fingerprint TEXT NOT NULL,
 fee_usd NUMERIC(38,18) NOT NULL CHECK(fee_usd>=0),settled_at TIMESTAMPTZ NOT NULL,evidence TEXT NOT NULL,verified_by UUID NOT NULL,journal_id UUID,
 created_at TIMESTAMPTZ NOT NULL
);
CREATE TRIGGER cooperation_procurement_refund_accounting_immutable BEFORE UPDATE OR DELETE ON cooperation_procurement_refund_accounting FOR EACH ROW EXECUTE FUNCTION ledger_immutable_fact();
