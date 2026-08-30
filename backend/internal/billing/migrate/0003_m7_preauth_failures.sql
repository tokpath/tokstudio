-- 预授权失败要落库，才能进运营看板。失败记录在事务外写入，避免被回滚抹掉。
CREATE TABLE IF NOT EXISTS billing_preauth_failures (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    request_id TEXT,
    reason TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS billing_preauth_failures_created_idx
    ON billing_preauth_failures (created_at DESC);
