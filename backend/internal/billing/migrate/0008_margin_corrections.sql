-- 毛利更正票：只记补成本 / 调毛利工单，不写估算成本，不双写 attempt 成本事实。
CREATE TABLE IF NOT EXISTS billing_margin_corrections (
    id TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    request_id TEXT NOT NULL,
    attempt_id TEXT,
    reason TEXT,
    status TEXT NOT NULL,
    actor_user_id TEXT NOT NULL,
    idempotency_key TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS billing_margin_corrections_request_idx
    ON billing_margin_corrections (request_id, created_at DESC);
