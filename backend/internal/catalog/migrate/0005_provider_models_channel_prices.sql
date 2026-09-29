-- 上游目录与报价属于提供商；渠道价格属于渠道对公开模型的授权。
CREATE TABLE IF NOT EXISTS catalog_provider_models (
    provider_id TEXT NOT NULL REFERENCES catalog_providers(id),
    upstream_model_id TEXT NOT NULL,
    display_name TEXT NOT NULL DEFAULT '',
    unit_costs_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    price_source TEXT NOT NULL DEFAULT 'unknown',
    discovered_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (provider_id, upstream_model_id)
);

ALTER TABLE catalog_channel_model_policies
    ADD COLUMN IF NOT EXISTS wholesale_json JSONB DEFAULT '{}'::jsonb;
ALTER TABLE catalog_channel_model_policies
    ADD COLUMN IF NOT EXISTS customer_override_json JSONB DEFAULT '{}'::jsonb;

-- Prelaunch catalog prices now contain only the public selling price. Usage
-- snapshots are left intact because settled bills must remain auditable.
UPDATE catalog_price_versions SET unit_prices_json = unit_prices_json
    - 'upstream_cost' - 'upstream_cost_input' - 'upstream_cost_output' - 'upstream_cost_reasoning'
    - 'wholesale' - 'wholesale_input' - 'wholesale_output'
    - 'channel_override' - 'channel_customer' - 'channel_customer_input' - 'channel_customer_output'
    - 'channel_customer_price_input' - 'channel_customer_price_output'
    - 'video_second_cost' - 'image_count_cost' - 'audio_second_cost';
DELETE FROM catalog_price_versions WHERE provider_id IS NOT NULL;
