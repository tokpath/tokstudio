-- API Key 过期与最后使用时间。轮换只改密文/hash，不换主键。
ALTER TABLE identity_api_keys ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;
ALTER TABLE identity_api_keys ADD COLUMN IF NOT EXISTS last_used_at TIMESTAMPTZ;
