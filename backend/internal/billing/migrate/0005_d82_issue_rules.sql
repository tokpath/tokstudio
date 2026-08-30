-- D8.2：平台按渠道配置“充值金额 -> 服务额度”换算比。
-- issue_ratio_bps：10000 = 1.0（默认 1:1），1000 = 0.1x，100000 = 10x。
-- 无行时按 10000 处理，不改现有 1:1 发放。B/C 代理商不能改此表（只走管理 API）。

CREATE TABLE IF NOT EXISTS billing_quota_issue_rules (
    id TEXT PRIMARY KEY,
    channel_org_id TEXT NOT NULL UNIQUE,
    issue_ratio_bps BIGINT NOT NULL,
    version BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT billing_quota_issue_ratio_range CHECK (
        issue_ratio_bps >= 1000 AND issue_ratio_bps <= 100000
    )
);

CREATE INDEX IF NOT EXISTS billing_quota_issue_rules_channel_idx
    ON billing_quota_issue_rules (channel_org_id);
