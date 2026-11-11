-- Each recurring schedule has a stable subject followed by a cycle timestamp.
-- Read its latest outcome without scanning every successful historical cycle.
CREATE INDEX durable_jobs_schedule_outcome ON durable_jobs
 ((regexp_replace(business_key, ':[0-9]+$', '')), created_at DESC, id DESC)
 WHERE queue='portal-maintenance' AND (status='completed' OR exhausted_at IS NOT NULL);
