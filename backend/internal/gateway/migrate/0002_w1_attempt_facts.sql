-- W1-①：attempt 级透传事实。缺 usage/metadata 保持 NULL，禁止估算填空。
ALTER TABLE gateway_attempts ADD COLUMN IF NOT EXISTS fact_source TEXT;
ALTER TABLE gateway_attempts ADD COLUMN IF NOT EXISTS prompt_tokens INTEGER;
ALTER TABLE gateway_attempts ADD COLUMN IF NOT EXISTS completion_tokens INTEGER;
ALTER TABLE gateway_attempts ADD COLUMN IF NOT EXISTS total_tokens INTEGER;
ALTER TABLE gateway_attempts ADD COLUMN IF NOT EXISTS metadata_json JSONB;
