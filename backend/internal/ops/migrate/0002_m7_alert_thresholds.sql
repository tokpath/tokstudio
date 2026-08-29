-- 告警阈值单独存，看板评估时读取，不写死在代码里。
CREATE TABLE IF NOT EXISTS ops_alert_thresholds (
    id TEXT PRIMARY KEY,
    success_rate_min DOUBLE PRECISION NOT NULL DEFAULT 0.5,
    min_requests INTEGER NOT NULL DEFAULT 5,
    pending_count INTEGER NOT NULL DEFAULT 1,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO ops_alert_thresholds (id, success_rate_min, min_requests, pending_count)
VALUES ('thr_default', 0.5, 5, 1)
ON CONFLICT (id) DO NOTHING;
