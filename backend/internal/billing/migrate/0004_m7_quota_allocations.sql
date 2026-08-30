-- B/C 用户充值后，按 1:1 从渠道可用额度发放服务额度。
-- 发放时扣渠道 available；请求结算只增加 consumed，不再二次扣渠道。

CREATE TABLE IF NOT EXISTS billing_quota_allocations (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    channel_org_id TEXT NOT NULL,
    source_type TEXT NOT NULL,
    source_id TEXT NOT NULL,
    granted_minor BIGINT NOT NULL,
    consumed_minor BIGINT NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'active',
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (source_type, source_id),
    CONSTRAINT billing_quota_alloc_nonneg CHECK (
        granted_minor >= 0 AND consumed_minor >= 0 AND consumed_minor <= granted_minor
    )
);

CREATE INDEX IF NOT EXISTS billing_quota_alloc_user_idx
    ON billing_quota_allocations (user_id, channel_org_id, created_at);

CREATE INDEX IF NOT EXISTS billing_quota_alloc_channel_idx
    ON billing_quota_allocations (channel_org_id, created_at DESC);

-- 每次结算从哪几笔发放里扣了多少，退消费账单时按 request_id 冲回。
CREATE TABLE IF NOT EXISTS billing_quota_consumes (
    id TEXT PRIMARY KEY,
    allocation_id TEXT NOT NULL REFERENCES billing_quota_allocations(id),
    request_id TEXT NOT NULL,
    amount_minor BIGINT NOT NULL,
    idempotency_key TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS billing_quota_consumes_request_idx
    ON billing_quota_consumes (request_id);
