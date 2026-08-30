-- 网关模块记录客户请求和每次上游 attempt。账务模块后续只消费这些公开事件。
CREATE TABLE IF NOT EXISTS gateway_requests (
    id TEXT PRIMARY KEY,
    request_id TEXT NOT NULL UNIQUE,
    user_id TEXT NOT NULL,
    api_key_id TEXT,
    channel_org_id TEXT,
    public_model_id TEXT NOT NULL,
    protocol TEXT NOT NULL,
    status TEXT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    ended_at TIMESTAMPTZ,
    final_attempt_id TEXT
);

CREATE TABLE IF NOT EXISTS gateway_attempts (
    id TEXT PRIMARY KEY,
    request_pk TEXT NOT NULL REFERENCES gateway_requests(id),
    provider_id TEXT NOT NULL,
    upstream_model_id TEXT NOT NULL,
    attempt_no INTEGER NOT NULL,
    status TEXT NOT NULL,
    http_status INTEGER,
    error_code TEXT,
    latency_ms INTEGER,
    started_at TIMESTAMPTZ NOT NULL,
    ended_at TIMESTAMPTZ
);
