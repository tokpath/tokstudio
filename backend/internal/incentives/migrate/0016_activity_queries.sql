CREATE INDEX incentives_campaign_versions_available ON incentives_campaign_versions (package_id,ends_at,starts_at) WHERE enabled;
CREATE INDEX incentives_order_snapshots_customer_activity ON incentives_order_snapshots (customer_id,status) WHERE merchant_version_id IS NOT NULL OR kol_version_id IS NOT NULL;
