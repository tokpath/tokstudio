ALTER TABLE payment_method_versions ADD COLUMN payout_currency TEXT NOT NULL DEFAULT '';
ALTER TABLE payment_method_versions ADD COLUMN payout_rate NUMERIC(38,18) NOT NULL DEFAULT 0;
ALTER TABLE payment_method_versions ADD COLUMN payout_note TEXT NOT NULL DEFAULT '';
ALTER TABLE payment_method_versions ADD COLUMN payout_networks JSONB NOT NULL DEFAULT '[]';
