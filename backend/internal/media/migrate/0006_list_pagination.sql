CREATE INDEX IF NOT EXISTS media_jobs_created_id_idx ON media_jobs (created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS media_jobs_user_created_id_idx ON media_jobs (user_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS media_jobs_channel_created_id_idx ON media_jobs (channel_org_id, created_at DESC, id DESC);
