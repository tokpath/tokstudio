CREATE INDEX IF NOT EXISTS tokenhub_model_active_promotion
ON tokenhub_model_price_versions (scope_key, public_model_id, ends_at, effective_at)
WHERE layer='promotion' AND definition->>'published'='true';
