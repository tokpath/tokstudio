-- Publish new configuration versions. Accepted order/withdrawal snapshots stay unchanged.
CREATE TEMP TABLE method_capability_changes ON COMMIT DROP AS
SELECT r.*,CASE WHEN r.method_kind='bank_card' THEN 'stripe'
 WHEN r.method_kind='cash' AND EXISTS(SELECT 1 FROM payment_method_identities i WHERE i.operator_id=r.operator_id AND i.kind='offline') THEN ''
 WHEN r.method_kind='cash' THEN 'offline' ELSE r.method_kind END AS target_kind
FROM (SELECT DISTINCT ON(operator_id,method_id) * FROM payment_method_versions ORDER BY operator_id,method_id,version DESC) r
WHERE r.method_kind IN ('bank_card','cash','offline','alipay','wechat','usdt');
DELETE FROM payment_method_identities i USING method_capability_changes c WHERE i.operator_id=c.operator_id AND i.method_id=c.method_id AND c.target_kind='';
UPDATE payment_method_identities i SET kind=c.target_kind FROM method_capability_changes c WHERE i.operator_id=c.operator_id AND i.method_id=c.method_id AND c.target_kind<>'';
INSERT INTO payment_method_versions(id,method_id,operator_id,version,name,collection,payout_enabled,payout_kind,flat_fee_money,fee_rate,created_by,method_kind,paused,payout_currency,payout_rate,payout_note,payout_networks)
SELECT gen_random_uuid(),method_id,operator_id,version+1,
 CASE target_kind WHEN 'stripe' THEN 'Stripe' WHEN 'offline' THEN '线下支付' ELSE name END,
 collection,CASE WHEN target_kind IN ('stripe','usdt','alipay','wechat','') THEN false ELSE payout_enabled END,
 CASE target_kind WHEN 'stripe' THEN '' WHEN 'offline' THEN 'offline' ELSE payout_kind END,
 flat_fee_money,fee_rate,'migration:method-capabilities',target_kind,paused OR target_kind='',
 CASE target_kind WHEN 'usdt' THEN 'USDT' WHEN 'alipay' THEN 'CNY' WHEN 'wechat' THEN 'CNY' ELSE payout_currency END,
 payout_rate,payout_note,payout_networks FROM method_capability_changes;
