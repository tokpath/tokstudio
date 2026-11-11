-- Crypto payment support is retired before launch. Preserve ordinary balances
-- and completed ledger facts; remove crypto withdrawal requests and release only
-- their still-active reservations. No external transfer is initiated here.
LOCK TABLE incentives_withdrawals, ledger_holds, ledger_accounts, incentives_events IN SHARE ROW EXCLUSIVE MODE;

CREATE TEMP TABLE retired_crypto_methods ON COMMIT DROP AS
SELECT DISTINCT operator_id, method_id FROM (
 SELECT DISTINCT ON(operator_id,method_id) * FROM payment_method_versions
 ORDER BY operator_id,method_id,version DESC
) r WHERE method_kind='usdt' OR payout_kind='usdt'
UNION
SELECT operator_id,payment_method_id FROM payments_merchants
WHERE driver IN ('evm_wallet','nowpayments');

CREATE TEMP TABLE retired_crypto_merchants ON COMMIT DROP AS
SELECT m.id FROM payments_merchants m WHERE m.driver IN ('evm_wallet','nowpayments')
 OR EXISTS(SELECT 1 FROM retired_crypto_methods r WHERE r.operator_id=m.operator_id AND r.method_id=m.payment_method_id);

CREATE TEMP TABLE retired_crypto_withdrawals ON COMMIT DROP AS
SELECT w.id,w.hold_id FROM incentives_withdrawals w
JOIN payment_method_versions r ON r.id=w.policy_id
WHERE r.payout_kind='usdt' OR r.method_kind='usdt'
 OR w.payout_currency IN ('USDT','USDC','BTC','ETH');

UPDATE ledger_accounts a SET held=a.held-x.amount,version=a.version+1
FROM (
 SELECT allocation.account_id,sum(allocation.amount) AS amount
 FROM retired_crypto_withdrawals w
 JOIN ledger_holds h ON h.id=w.hold_id AND h.status='active'
 JOIN ledger_hold_allocations allocation ON allocation.hold_id=h.id
 GROUP BY allocation.account_id
) x WHERE a.id=x.account_id;
UPDATE ledger_holds h SET status='released',resolution_key='migration:remove-crypto-payments',resolved_at=clock_timestamp()
FROM retired_crypto_withdrawals w WHERE h.id=w.hold_id AND h.status='active';
-- The pre-launch cleanup explicitly includes obsolete withdrawal audit rows.
-- Suspend only their immutability trigger inside this migration transaction;
-- ordinary events remain protected once the transaction commits (or rolls back).
ALTER TABLE incentives_events DISABLE TRIGGER incentives_audit_immutable;
DELETE FROM incentives_events e USING retired_crypto_withdrawals w WHERE e.aggregate_id=w.id;
ALTER TABLE incentives_events ENABLE TRIGGER incentives_audit_immutable;
DELETE FROM incentives_withdrawals w USING retired_crypto_withdrawals r WHERE w.id=r.id;

INSERT INTO payment_method_versions(id,method_id,operator_id,version,name,collection,payout_enabled,payout_kind,flat_fee_money,fee_rate,created_by,method_kind,paused)
SELECT gen_random_uuid(),r.method_id,r.operator_id,r.version+1,r.name,false,false,r.payout_kind,r.flat_fee_money,r.fee_rate,'migration:remove-crypto-payments',r.method_kind,true
FROM (SELECT DISTINCT ON(operator_id,method_id) * FROM payment_method_versions ORDER BY operator_id,method_id,version DESC) r
JOIN retired_crypto_methods c ON c.operator_id=r.operator_id AND c.method_id=r.method_id
WHERE NOT r.paused OR r.collection OR r.payout_enabled;
DELETE FROM payment_method_identities i USING retired_crypto_methods c WHERE i.operator_id=c.operator_id AND i.method_id=c.method_id;

UPDATE payments_merchants SET enabled=false,definition=jsonb_set(definition,'{enabled}','false')
WHERE id IN (SELECT id FROM retired_crypto_merchants);
DELETE FROM payments_webhook_inbox i USING retired_crypto_merchants m WHERE i.merchant_id=m.id;
UPDATE payments_intents i SET state='cancelled',checkout_ciphertext=NULL,updated_at=clock_timestamp()
FROM retired_crypto_merchants m WHERE i.merchant_id=m.id
AND i.state IN ('executing','created','outcome_unknown');
UPDATE payments_refund_intents i SET state='failed',updated_at=clock_timestamp()
FROM retired_crypto_merchants m WHERE i.merchant_id=m.id
AND i.state NOT IN ('succeeded','failed');

DROP TABLE IF EXISTS payments_asset_network_versions;
DROP FUNCTION IF EXISTS payments_immutable_asset_version();
