-- A stored default or manual availability flag is not a successful health check.
ALTER TABLE catalog_providers ADD COLUMN health_checked_at TIMESTAMPTZ;
