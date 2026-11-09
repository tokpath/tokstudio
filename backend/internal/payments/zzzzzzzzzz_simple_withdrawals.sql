-- Remove the extra payout configuration without rewriting accepted money facts.
-- Restore methods only if the last change was our earlier migration; respect later operator edits.
WITH current_rules AS (
 SELECT DISTINCT ON(operator_id,method_id) * FROM payment_method_versions
 ORDER BY operator_id,method_id,version DESC
)
INSERT INTO payment_method_versions(id,method_id,operator_id,version,name,collection,payout_enabled,payout_kind,flat_fee_money,fee_rate,created_by,method_kind,paused)
SELECT gen_random_uuid(),r.method_id,r.operator_id,r.version+1,r.name,r.collection,
 CASE WHEN r.created_by='migration:method-capabilities' AND r.method_kind IN ('alipay','wechat','usdt')
 THEN COALESCE((SELECT p.payout_enabled FROM payment_method_versions p WHERE p.operator_id=r.operator_id AND p.method_id=r.method_id AND p.version<r.version ORDER BY p.version DESC LIMIT 1),false)
 ELSE r.payout_enabled END,
 r.payout_kind,r.flat_fee_money,r.fee_rate,'migration:simple-withdrawal-rules',r.method_kind,r.paused
FROM current_rules r
WHERE r.payout_currency<>'' OR r.payout_rate<>0 OR r.payout_note<>'' OR r.payout_networks<>'[]'::jsonb;
