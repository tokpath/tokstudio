-- Do not strand an actual pending transfer or a receipt reservation. Sites
-- with pre-launch obsolete financial cases must clear those scoped fixtures first.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM ledger_holds WHERE purpose='procurement_exit_refund' AND status='active')
 OR EXISTS(SELECT 1 FROM ledger_accounts WHERE purpose IN ('procurement_exit:payable','procurement_exit:receivable') AND balance<>0)
 THEN RAISE EXCEPTION 'retire unresolved pre-launch procurement exit fixtures before removing this workflow'; END IF;
END $$;
-- All procurement is nonrefundable. Retire the unused pre-launch exit workflow.
DROP TABLE cooperation_refund_manual_failures, cooperation_refund_route_closures,
 cooperation_refund_destination_authorizations, cooperation_refund_destination_proposals,
 cooperation_manual_receipt_refund_facts, cooperation_procurement_refund_instructions,
 cooperation_procurement_refund_observations, cooperation_procurement_refund_accounting,
 cooperation_procurement_exit_refunds, cooperation_procurement_exits, cooperation_supply_failure_approvals;
ALTER TABLE cooperation_purchase_quotes DISABLE TRIGGER USER;
UPDATE cooperation_purchase_quotes SET exit_terms=jsonb_build_object('version',1,'voluntary_refund',false,'method','non_refundable','terms_reference','procurement-exit:'||id::text);
ALTER TABLE cooperation_purchase_quotes ENABLE TRIGGER USER;
ALTER TABLE cooperation_purchase_orders DISABLE TRIGGER USER;
UPDATE cooperation_purchase_orders SET exit_terms=jsonb_build_object('version',1,'voluntary_refund',false,'method','non_refundable','terms_reference','procurement-exit:'||id::text);
ALTER TABLE cooperation_purchase_orders ENABLE TRIGGER USER;
ALTER TABLE cooperation_procurement_lots DISABLE TRIGGER USER;
UPDATE cooperation_procurement_lots SET exit_terms=jsonb_build_object('version',1,'voluntary_refund',false,'method','non_refundable','terms_reference','procurement-exit:'||id::text);
ALTER TABLE cooperation_procurement_lots ENABLE TRIGGER USER;
