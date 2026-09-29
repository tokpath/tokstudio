ALTER TABLE catalog_provider_models
    ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active';
