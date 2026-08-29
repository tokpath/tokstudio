-- 一个 Provider 可挂多条上游账号。明文密钥仍只存在 ciphertext。
ALTER TABLE catalog_provider_credentials ADD COLUMN IF NOT EXISTS kind TEXT NOT NULL DEFAULT 'api_key';
ALTER TABLE catalog_provider_credentials ADD COLUMN IF NOT EXISTS label TEXT NOT NULL DEFAULT '';
ALTER TABLE catalog_provider_credentials ADD COLUMN IF NOT EXISTS model_tags TEXT NOT NULL DEFAULT '';
ALTER TABLE catalog_provider_credentials ADD COLUMN IF NOT EXISTS rpm_limit INTEGER NOT NULL DEFAULT 0;
ALTER TABLE catalog_provider_credentials ADD COLUMN IF NOT EXISTS concurrency_limit INTEGER NOT NULL DEFAULT 0;
ALTER TABLE catalog_provider_credentials ADD COLUMN IF NOT EXISTS last_success_at TIMESTAMPTZ;
ALTER TABLE catalog_provider_credentials ADD COLUMN IF NOT EXISTS last_error_at TIMESTAMPTZ;
ALTER TABLE catalog_provider_credentials ADD COLUMN IF NOT EXISTS last_error_code TEXT NOT NULL DEFAULT '';
ALTER TABLE catalog_provider_credentials ADD COLUMN IF NOT EXISTS cooldown_until TIMESTAMPTZ;
ALTER TABLE catalog_provider_credentials ADD COLUMN IF NOT EXISTS rotated_at TIMESTAMPTZ;
