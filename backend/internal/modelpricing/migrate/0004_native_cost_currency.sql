-- Actual supplier statements keep the currency and amount on the statement.
ALTER TABLE model_actual_costs RENAME COLUMN amount_money TO amount;
ALTER TABLE model_actual_costs ADD COLUMN currency TEXT NOT NULL DEFAULT 'USD' CHECK(currency ~ '^[A-Z]{2,12}$');
ALTER TABLE model_actual_costs ALTER COLUMN currency DROP DEFAULT;
CREATE INDEX model_actual_costs_recorded_currency ON model_actual_costs(created_at,currency);
