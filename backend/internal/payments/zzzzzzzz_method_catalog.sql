ALTER TABLE payments_merchants ADD COLUMN payment_method_id TEXT NOT NULL DEFAULT '';
UPDATE payments_merchants SET payment_method_id=id;
-- Only current configuration changes. Submitted withdrawals, accepted orders,
-- receipt facts and the encrypted historical merchant versions stay intact.
CREATE TEMP TABLE method_catalog_candidates ON COMMIT DROP AS
WITH latest AS (SELECT DISTINCT ON(operator_id,method_id) * FROM payment_method_versions ORDER BY operator_id,method_id,version DESC),
classified AS (
 SELECT r.*,m.id AS connection_id,
 CASE m.driver WHEN 'stripe' THEN 'bank_card' WHEN 'manual' THEN 'offline' WHEN 'alipay' THEN 'alipay' WHEN 'wechat' THEN 'wechat' WHEN 'kvoucher' THEN 'kvoucher' WHEN 'nowpayments' THEN 'usdt' WHEN 'evm_wallet' THEN 'usdt'
 ELSE CASE r.payout_kind WHEN 'bank_cn' THEN 'bank_transfer' WHEN 'alipay' THEN 'alipay' WHEN 'wechat' THEN 'wechat' WHEN 'paypal' THEN 'paypal' WHEN 'wire' THEN 'wire' WHEN 'usdt' THEN 'usdt' ELSE '' END END AS inferred_kind,
 COALESCE(m.enabled,false) AS connection_enabled
 FROM latest r LEFT JOIN payments_merchants m ON m.id=r.method_id AND m.operator_id=r.operator_id
)
SELECT *,row_number() OVER(PARTITION BY operator_id,inferred_kind ORDER BY connection_enabled DESC,collection DESC,created_at DESC,method_id) AS preference FROM classified;
INSERT INTO payment_method_identities(operator_id,kind,method_id,merchant_id)
SELECT operator_id,inferred_kind,method_id,COALESCE(connection_id,'') FROM method_catalog_candidates WHERE preference=1 AND inferred_kind<>'';
INSERT INTO payment_method_versions(id,method_id,operator_id,version,name,collection,payout_enabled,payout_kind,flat_fee_money,fee_rate,created_by,method_kind,paused)
SELECT gen_random_uuid(),method_id,operator_id,version+1,
 CASE WHEN preference=1 THEN CASE inferred_kind WHEN 'offline' THEN '线下支付' WHEN 'bank_card' THEN '银行卡' WHEN 'bank_transfer' THEN '银行转账' WHEN 'alipay' THEN '支付宝' WHEN 'wechat' THEN '微信支付' WHEN 'paypal' THEN 'PayPal' WHEN 'wire' THEN '国际电汇' WHEN 'usdt' THEN 'USDT' WHEN 'kvoucher' THEN 'KVoucher' ELSE name END ELSE name END,
 collection,payout_enabled,payout_kind,flat_fee_money,fee_rate,'migration:method-catalog',
 CASE WHEN preference=1 THEN inferred_kind ELSE '' END,
 preference<>1 OR inferred_kind=''
FROM method_catalog_candidates;
