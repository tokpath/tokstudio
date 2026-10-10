-- Correct an erroneous registration without claiming an external cash refund.
-- Original amounts remain intact; reversal metadata and quota ledger are atomic.
ALTER TABLE billing_oem_purchases ADD COLUMN status TEXT NOT NULL DEFAULT 'completed' CHECK (status IN ('completed','reversed'));
ALTER TABLE billing_oem_purchases ADD COLUMN reversal_operation_id TEXT NOT NULL DEFAULT '';
ALTER TABLE billing_oem_purchases ADD COLUMN reversed_by TEXT NOT NULL DEFAULT '';
ALTER TABLE billing_oem_purchases ADD COLUMN reversal_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE billing_oem_purchases ADD COLUMN reversed_at TIMESTAMPTZ;
ALTER TABLE billing_oem_purchases ADD COLUMN reversal_result_json JSONB;
CREATE UNIQUE INDEX billing_oem_purchase_reversal_operation ON billing_oem_purchases(reversal_operation_id) WHERE reversal_operation_id <> '';
DROP INDEX billing_oem_purchase_external_unique;
CREATE UNIQUE INDEX billing_oem_purchase_external_unique ON billing_oem_purchases(external_reference) WHERE external_reference <> '' AND status = 'completed';
