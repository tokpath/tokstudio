-- Costs are the accepted candidate's original terms. Legacy jobs remain
-- without a supplier snapshot; never invent one from current route prices.
ALTER TABLE media_jobs ADD COLUMN supplier_costs_json JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE media_jobs ADD COLUMN upstream_model_id TEXT NOT NULL DEFAULT '';
