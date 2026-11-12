ALTER TABLE commerce_receipts ADD COLUMN settlement_currency TEXT NOT NULL DEFAULT '';
ALTER TABLE commerce_receipts ADD COLUMN settlement_net_amount NUMERIC(38,18);
-- Preserve already verified original-currency net receipts without inventing
-- a value for records whose provider settlement was never known.
UPDATE commerce_receipts SET settlement_currency=currency,settlement_net_amount=original_net_amount
 WHERE original_net_amount IS NOT NULL AND currency ~ '^[A-Z]{3}$';
ALTER TABLE commerce_receipts ADD CONSTRAINT receipt_native_settlement CHECK (
 (settlement_net_amount IS NULL AND settlement_currency='') OR
 (settlement_net_amount>=0 AND settlement_currency ~ '^[A-Z]{3}$')
);
-- Order funding and provider settlement can differ even in the same currency.
DO $$ DECLARE c RECORD; BEGIN
 FOR c IN SELECT conname FROM pg_constraint WHERE conrelid='commerce_receipts'::regclass
 AND contype='c' AND pg_get_constraintdef(oid) LIKE '%fee_status%'
 LOOP EXECUTE format('ALTER TABLE commerce_receipts DROP CONSTRAINT %I',c.conname); END LOOP;
END $$;
ALTER TABLE commerce_receipts ADD CONSTRAINT receipt_fee_state CHECK (
 (fee_status='pending' AND net_money IS NULL AND fee_money IS NULL AND settled_at IS NULL) OR
 (fee_status='confirmed' AND net_money IS NOT NULL AND fee_money IS NOT NULL)
);
