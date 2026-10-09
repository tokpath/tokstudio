ALTER TABLE billing_quota_allocations ADD COLUMN IF NOT EXISTS pool_channel_org_id TEXT NOT NULL DEFAULT '';
UPDATE billing_quota_allocations SET pool_channel_org_id = channel_org_id WHERE pool_channel_org_id = '';
CREATE INDEX IF NOT EXISTS billing_alloc_pool_idx ON billing_quota_allocations(pool_channel_org_id, created_at DESC);
