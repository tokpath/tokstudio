-- Business tasks are durable PostgreSQL facts. Redis is only a wake-up hint.
CREATE TABLE durable_jobs (
    id TEXT PRIMARY KEY,
    queue TEXT NOT NULL,
    kind TEXT NOT NULL,
    business_key TEXT NOT NULL,
    payload JSONB NOT NULL,
    payload_hash TEXT NOT NULL CHECK (length(payload_hash) = 64),
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','running','completed','dead')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    max_attempts INTEGER NOT NULL CHECK (max_attempts BETWEEN 1 AND 100),
    available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    lease_until TIMESTAMPTZ,
    lease_owner TEXT NOT NULL DEFAULT '',
    fence BIGINT NOT NULL DEFAULT 0 CHECK (fence >= 0),
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    UNIQUE (queue,kind,business_key),
    CHECK ((status = 'running') = (lease_until IS NOT NULL AND lease_owner <> ''))
);
CREATE INDEX durable_jobs_ready_idx ON durable_jobs(queue,available_at,created_at,id) WHERE status = 'pending';
CREATE INDEX durable_jobs_expired_idx ON durable_jobs(queue,lease_until) WHERE status = 'running';
CREATE INDEX durable_jobs_dead_idx ON durable_jobs(queue,updated_at,id) WHERE status = 'dead';
