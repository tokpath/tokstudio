-- D3.2：视频多模式与参考素材。禁止 AutoMigrate，只走版本化 SQL。
-- task_type 默认 t2v，兼容已有文生视频行。images_json 存参考图 URL/引用，不存 prompt。

ALTER TABLE media_jobs
    ADD COLUMN IF NOT EXISTS task_type TEXT NOT NULL DEFAULT 't2v',
    ADD COLUMN IF NOT EXISTS first_frame TEXT,
    ADD COLUMN IF NOT EXISTS last_frame TEXT,
    ADD COLUMN IF NOT EXISTS reference_video TEXT,
    ADD COLUMN IF NOT EXISTS reference_audio TEXT,
    ADD COLUMN IF NOT EXISTS source_job_id TEXT,
    ADD COLUMN IF NOT EXISTS images_json JSONB NOT NULL DEFAULT '[]'::jsonb;

CREATE INDEX IF NOT EXISTS media_jobs_task_type_idx ON media_jobs (task_type, created_at);
CREATE INDEX IF NOT EXISTS media_jobs_source_job_idx ON media_jobs (source_job_id)
    WHERE source_job_id IS NOT NULL AND source_job_id <> '';
