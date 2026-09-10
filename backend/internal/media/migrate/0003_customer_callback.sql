-- 客户 callback_url 投递状态。禁止 AutoMigrate。
ALTER TABLE media_jobs
    ADD COLUMN IF NOT EXISTS callback_event_id TEXT,
    ADD COLUMN IF NOT EXISTS callback_status TEXT,
    ADD COLUMN IF NOT EXISTS callback_attempts INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS callback_next_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS callback_last_error TEXT;

CREATE INDEX IF NOT EXISTS media_jobs_callback_due_idx
    ON media_jobs (callback_status, callback_next_at)
    WHERE callback_status = 'pending';
