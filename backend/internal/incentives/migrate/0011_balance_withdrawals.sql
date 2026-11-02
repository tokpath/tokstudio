ALTER TABLE incentives_withdrawal_policies
  ADD COLUMN wallet TEXT NOT NULL DEFAULT 'commission';
ALTER TABLE incentives_withdrawal_policies
  ADD CONSTRAINT incentives_withdrawal_policies_wallet_check
  CHECK (wallet IN ('commission', 'balance'));

ALTER TABLE incentives_withdrawals
  ADD COLUMN wallet TEXT NOT NULL DEFAULT 'commission';
ALTER TABLE incentives_withdrawals
  ADD CONSTRAINT incentives_withdrawals_wallet_check
  CHECK (wallet IN ('commission', 'balance'));

ALTER TABLE incentives_withdrawal_settings
  ADD COLUMN wallet TEXT NOT NULL DEFAULT 'commission';
ALTER TABLE incentives_withdrawal_settings
  DROP CONSTRAINT incentives_withdrawal_settings_pkey;
ALTER TABLE incentives_withdrawal_settings
  ADD PRIMARY KEY (operator_id, wallet);
ALTER TABLE incentives_withdrawal_settings
  ADD CONSTRAINT incentives_withdrawal_settings_wallet_check
  CHECK (wallet IN ('commission', 'balance'));
