-- 运营模块自有表。禁止直连 billing/gateway/identity 业务表。
CREATE TABLE IF NOT EXISTS ops_runbooks (
    id TEXT PRIMARY KEY,
    alert_kind TEXT NOT NULL UNIQUE,
    title TEXT NOT NULL,
    body TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ops_alerts (
    id TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    severity TEXT NOT NULL,
    status TEXT NOT NULL,
    message TEXT NOT NULL,
    payload_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS ops_backup_drills (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL,
    rpo_minutes INTEGER NOT NULL,
    rto_minutes INTEGER NOT NULL,
    method TEXT NOT NULL,
    evidence TEXT NOT NULL,
    actor_user_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ops_canary (
    id TEXT PRIMARY KEY,
    route_key TEXT NOT NULL UNIQUE,
    provider_slug TEXT NOT NULL,
    percent INTEGER NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS ops_alerts_kind_idx ON ops_alerts (kind, status, created_at DESC);
CREATE INDEX IF NOT EXISTS ops_drills_created_idx ON ops_backup_drills (created_at DESC);
