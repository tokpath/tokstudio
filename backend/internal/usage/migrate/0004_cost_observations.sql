ALTER TABLE usage_attempts ADD COLUMN supplier_model_id TEXT NOT NULL DEFAULT '';
ALTER TABLE usage_attempts ADD COLUMN cost_observation JSONB;
ALTER TABLE usage_attempts ADD COLUMN calculated_cost_usd NUMERIC(38,18);
CREATE INDEX usage_attempt_model_period ON usage_attempts(provider_id,supplier_model_id,started_at);
