-- 目录模块：Provider、公开模型、映射、价格、路由。其他模块不得直连这些表。
CREATE TABLE IF NOT EXISTS catalog_providers (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    kind TEXT NOT NULL,
    adapter TEXT NOT NULL,
    base_url TEXT,
    region TEXT,
    status TEXT NOT NULL DEFAULT 'active',
    health TEXT NOT NULL DEFAULT 'available',
    test_behavior TEXT NOT NULL DEFAULT 'ok',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS catalog_provider_credentials (
    id TEXT PRIMARY KEY,
    provider_id TEXT NOT NULL REFERENCES catalog_providers(id),
    ciphertext TEXT NOT NULL,
    key_hash TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS catalog_public_models (
    id TEXT PRIMARY KEY,
    public_id TEXT NOT NULL UNIQUE,
    vendor TEXT NOT NULL,
    display_name TEXT NOT NULL,
    capabilities_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL DEFAULT 'published'
);

CREATE TABLE IF NOT EXISTS catalog_provider_model_mappings (
    id TEXT PRIMARY KEY,
    public_model_id TEXT NOT NULL REFERENCES catalog_public_models(id),
    provider_id TEXT NOT NULL REFERENCES catalog_providers(id),
    upstream_model_id TEXT NOT NULL,
    capabilities_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    sync_state TEXT NOT NULL DEFAULT 'published',
    status TEXT NOT NULL DEFAULT 'active'
);

CREATE TABLE IF NOT EXISTS catalog_price_versions (
    id TEXT PRIMARY KEY,
    public_model_id TEXT NOT NULL REFERENCES catalog_public_models(id),
    provider_id TEXT REFERENCES catalog_providers(id),
    unit_prices_json JSONB NOT NULL,
    effective_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    status TEXT NOT NULL DEFAULT 'published'
);

CREATE TABLE IF NOT EXISTS catalog_route_groups (
    id TEXT PRIMARY KEY,
    public_model_id TEXT NOT NULL REFERENCES catalog_public_models(id),
    strategy TEXT NOT NULL DEFAULT 'priority',
    fallback_policy TEXT NOT NULL DEFAULT 'same_model',
    status TEXT NOT NULL DEFAULT 'active'
);

CREATE TABLE IF NOT EXISTS catalog_route_candidates (
    route_group_id TEXT NOT NULL REFERENCES catalog_route_groups(id),
    provider_id TEXT NOT NULL REFERENCES catalog_providers(id),
    priority INTEGER NOT NULL,
    weight INTEGER NOT NULL DEFAULT 1,
    PRIMARY KEY (route_group_id, provider_id)
);

CREATE TABLE IF NOT EXISTS catalog_channel_model_policies (
    channel_org_id TEXT NOT NULL,
    public_model_id TEXT NOT NULL REFERENCES catalog_public_models(id),
    enabled BOOLEAN NOT NULL DEFAULT true,
    PRIMARY KEY (channel_org_id, public_model_id)
);
