-- media 模块自有表。其他模块禁止直连。
CREATE TABLE IF NOT EXISTS media_jobs (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    api_key_id TEXT,
    channel_org_id TEXT,
    request_id TEXT NOT NULL UNIQUE,
    public_model_id TEXT NOT NULL,
    provider_id TEXT,
    upstream_job_id TEXT,
    job_kind TEXT NOT NULL,
    status TEXT NOT NULL,
    progress INTEGER NOT NULL DEFAULT 0,
    prompt TEXT,
    duration_seconds INTEGER NOT NULL DEFAULT 5,
    resolution TEXT,
    aspect_ratio TEXT,
    fps INTEGER,
    generate_audio BOOLEAN NOT NULL DEFAULT false,
    callback_url TEXT,
    idempotency_key TEXT,
    error_code TEXT,
    usage_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS media_jobs_user_idem_idx
    ON media_jobs (user_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL AND idempotency_key <> '';

CREATE TABLE IF NOT EXISTS media_assets (
    id TEXT PRIMARY KEY,
    media_job_id TEXT NOT NULL REFERENCES media_jobs(id),
    kind TEXT NOT NULL,
    object_key TEXT NOT NULL,
    content_type TEXT NOT NULL,
    size_bytes BIGINT NOT NULL DEFAULT 0,
    sha256 TEXT,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS media_callback_events (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL UNIQUE,
    media_job_id TEXT,
    signature_valid BOOLEAN NOT NULL,
    payload_json JSONB NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS media_jobs_status_idx ON media_jobs (status, updated_at);
CREATE INDEX IF NOT EXISTS media_assets_expiry_idx ON media_assets (expires_at);
