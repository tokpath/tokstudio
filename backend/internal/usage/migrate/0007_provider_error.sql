ALTER TABLE usage_attempts ADD COLUMN provider_status INTEGER NOT NULL DEFAULT 0;
ALTER TABLE usage_attempts ADD COLUMN provider_error TEXT NOT NULL DEFAULT '';
