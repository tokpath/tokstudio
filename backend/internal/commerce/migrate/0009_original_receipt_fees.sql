ALTER TABLE commerce_receipts ADD COLUMN original_fee_amount NUMERIC(38,18),ADD COLUMN original_net_amount NUMERIC(38,18);
ALTER TABLE commerce_receipts ADD CONSTRAINT original_fee_integrity CHECK ((original_fee_amount IS NULL AND original_net_amount IS NULL) OR (original_fee_amount>=0 AND original_net_amount>=0 AND original_net_amount+original_fee_amount=original_amount::numeric));
