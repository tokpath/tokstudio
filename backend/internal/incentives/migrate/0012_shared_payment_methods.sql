-- Keep submitted withdrawal amounts and recipient snapshots. Incomplete or
-- obsolete combined fee configurations are disabled until configured again.
-- Each valid method now serves both wallets.
INSERT INTO payment_method_versions(id,method_id,operator_id,version,name,collection,payout_enabled,payout_kind,flat_fee_money,fee_rate,created_at)
SELECT id,method_id,operator_id,version,method_name,false,enabled AND payout_kind<>'' AND NOT(flat_fee_money>0 AND fee_rate>0),payout_kind,CASE WHEN flat_fee_money>0 AND fee_rate>0 THEN 0 ELSE flat_fee_money END,CASE WHEN flat_fee_money>0 AND fee_rate>0 THEN 0 ELSE fee_rate END,effective_at
FROM incentives_withdrawal_policies;
ALTER TABLE incentives_withdrawals DROP CONSTRAINT incentives_withdrawals_policy_id_fkey;
ALTER TABLE incentives_withdrawals ADD CONSTRAINT incentives_withdrawals_policy_id_fkey FOREIGN KEY(policy_id) REFERENCES payment_method_versions(id);
DROP TABLE incentives_withdrawal_policies;
