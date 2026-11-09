ALTER TABLE incentives_withdrawals ADD COLUMN payout_currency TEXT NOT NULL DEFAULT '';
ALTER TABLE incentives_withdrawals ADD COLUMN payout_amount NUMERIC(38,18) NOT NULL DEFAULT 0;
