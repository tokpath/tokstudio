-- Supplier observations are independent original-currency evidence, not
-- settlement money. Preserve the prior USD observations as their stated USD.
ALTER TABLE usage_attempts ADD COLUMN reference_cost_currency TEXT NOT NULL DEFAULT '';
ALTER TABLE usage_attempts ADD COLUMN reference_cost_amount NUMERIC CHECK(reference_cost_amount>=0);
ALTER TABLE usage_attempts ADD COLUMN reported_cost_currency TEXT NOT NULL DEFAULT '';
ALTER TABLE usage_attempts ADD COLUMN reported_cost_amount NUMERIC CHECK(reported_cost_amount>=0);
UPDATE usage_attempts SET reference_cost_currency='USD',reference_cost_amount=calculated_cost_money WHERE calculated_cost_money IS NOT NULL;
UPDATE usage_attempts SET reported_cost_currency='USD',reported_cost_amount=supplier_cost_money WHERE cost_observation->>'source'='provider_reported' AND cost_status='confirmed';
ALTER TABLE usage_attempts DROP COLUMN supplier_cost_money,DROP COLUMN calculated_cost_money;
ALTER TABLE usage_attempts ADD CONSTRAINT usage_reference_cost_currency CHECK((reference_cost_amount IS NULL AND reference_cost_currency='') OR (reference_cost_amount IS NOT NULL AND reference_cost_currency ~ '^[A-Z]{2,12}$'));
ALTER TABLE usage_attempts ADD CONSTRAINT usage_reported_cost_currency CHECK((reported_cost_amount IS NULL AND reported_cost_currency='') OR (reported_cost_amount IS NOT NULL AND reported_cost_currency ~ '^[A-Z]{2,12}$'));
CREATE INDEX usage_attempt_cost_currency ON usage_attempts(cost_recorded_at,reference_cost_currency,reported_cost_currency);
