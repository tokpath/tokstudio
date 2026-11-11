-- Keep the last exhausted attempt visible while the original task is retried.
ALTER TABLE durable_jobs ADD COLUMN exhausted_at TIMESTAMPTZ;
UPDATE durable_jobs SET exhausted_at=updated_at WHERE status='dead';
CREATE INDEX durable_jobs_attention ON durable_jobs(exhausted_at DESC,id DESC)
 WHERE exhausted_at IS NOT NULL AND status<>'completed';
