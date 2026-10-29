-- Settlement confirmations preserve one real transfer and its explicitly
-- verified order amount. Reusable FX quotes and conversion residuals are gone.
DROP TABLE payments_valuation_residuals;
UPDATE payments_verified_facts SET state='awaiting_settlement', valuation_reason='settlement_amount_unconfirmed'
WHERE state IN ('awaiting_valuation','awaiting_precision');
UPDATE payments_intents SET state='awaiting_settlement' WHERE state IN ('awaiting_valuation','awaiting_precision');
