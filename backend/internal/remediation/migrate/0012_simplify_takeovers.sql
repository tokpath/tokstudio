DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM ledger_accounts WHERE purpose IN ('counterparty:takeover_payable','counterparty:takeover_receivable') AND balance<>0)
 THEN RAISE EXCEPTION 'retire unresolved pre-launch takeover valuation fixtures before removing settlement'; END IF;
END $$;
-- Old preparation records used a different, now removed valuation agreement.
UPDATE remediation_takeovers SET state='cancelled' WHERE state IN ('preparing','awaiting_approval');
INSERT INTO organization_takeover_locks(operator_id,takeover_id)
 SELECT channel_id,id FROM remediation_takeovers WHERE state='completed' ON CONFLICT DO NOTHING;
UPDATE organization_operators SET lifecycle='terminated'
 WHERE id IN (SELECT channel_id FROM remediation_takeovers WHERE state='completed');
DROP TABLE remediation_takeover_settlements;
ALTER TABLE remediation_takeovers
 DROP COLUMN method, DROP COLUMN service_money_per_atg, DROP COLUMN procurement_money_per_atg,
 DROP COLUMN extra_money, DROP COLUMN service_money, DROP COLUMN procurement_offset_atg,
 DROP COLUMN procurement_offset_money, DROP COLUMN remaining_procurement_refund_money,
 DROP COLUMN settlement_due_money, DROP COLUMN settled_money;

UPDATE remediation_takeovers SET snapshot=NULL;
