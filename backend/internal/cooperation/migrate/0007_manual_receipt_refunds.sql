ALTER TABLE cooperation_procurement_exit_refunds ALTER COLUMN merchant_id DROP NOT NULL;
ALTER TABLE cooperation_procurement_exit_refunds ADD COLUMN source_kind TEXT NOT NULL DEFAULT 'payment' CHECK(source_kind IN('payment','manual_receipt'));
ALTER TABLE cooperation_procurement_exit_refunds ADD CHECK((source_kind='payment' AND merchant_id IS NOT NULL) OR (source_kind='manual_receipt' AND merchant_id IS NULL));
CREATE TABLE cooperation_manual_receipt_refund_facts(
 fact_id TEXT PRIMARY KEY,leg_id UUID NOT NULL REFERENCES cooperation_procurement_exit_refunds(id),
 execution_key TEXT NOT NULL,fingerprint TEXT NOT NULL,proof JSONB NOT NULL,valuation JSONB NOT NULL,
 valued_at TIMESTAMPTZ,fx_reference TEXT NOT NULL,verified_by UUID NOT NULL,created_at TIMESTAMPTZ NOT NULL, UNIQUE(leg_id,execution_key)
);
CREATE TRIGGER cooperation_manual_receipt_refunds_immutable BEFORE UPDATE OR DELETE ON cooperation_manual_receipt_refund_facts FOR EACH ROW EXECUTE FUNCTION ledger_immutable_fact();
