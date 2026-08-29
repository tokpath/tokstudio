-- plans 模块自有表。billing/payment 禁止直连。
CREATE TABLE IF NOT EXISTS plans_product_plans (
    id TEXT PRIMARY KEY,
    owner_type TEXT NOT NULL,
    owner_id TEXT NOT NULL,
    name TEXT NOT NULL,
    currency TEXT NOT NULL DEFAULT 'USD',
    price_minor BIGINT NOT NULL,
    billing_period TEXT NOT NULL DEFAULT 'monthly',
    status TEXT NOT NULL,
    policy_version TEXT NOT NULL DEFAULT 'm5-v1',
    auto_renew_allowed BOOLEAN NOT NULL DEFAULT true,
    review_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS plans_plan_items (
    id TEXT PRIMARY KEY,
    plan_id TEXT NOT NULL REFERENCES plans_product_plans(id),
    public_model_id TEXT,
    unit_type TEXT NOT NULL,
    included_amount BIGINT NOT NULL,
    overage_price_minor BIGINT NOT NULL DEFAULT 0,
    expires_in_seconds INTEGER NOT NULL DEFAULT 2592000
);

CREATE TABLE IF NOT EXISTS plans_subscriptions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    channel_org_id TEXT,
    plan_id TEXT NOT NULL REFERENCES plans_product_plans(id),
    status TEXT NOT NULL,
    current_period_start TIMESTAMPTZ,
    current_period_end TIMESTAMPTZ,
    renewal_policy TEXT NOT NULL DEFAULT 'auto',
    payment_adapter TEXT,
    payment_method_ref TEXT,
    retry_count INTEGER NOT NULL DEFAULT 0,
    next_retry_at TIMESTAMPTZ,
    grace_until TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS plans_entitlement_accounts (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    source_type TEXT NOT NULL,
    source_id TEXT NOT NULL,
    unit_type TEXT NOT NULL,
    public_model_id TEXT,
    granted BIGINT NOT NULL,
    consumed BIGINT NOT NULL DEFAULT 0,
    expires_at TIMESTAMPTZ,
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS plans_entitlement_ledger (
    id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES plans_entitlement_accounts(id),
    event_type TEXT NOT NULL,
    amount BIGINT NOT NULL,
    request_id TEXT,
    idempotency_key TEXT NOT NULL UNIQUE,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS plans_entitlement_user_idx
    ON plans_entitlement_accounts (user_id, unit_type, status, expires_at);
CREATE INDEX IF NOT EXISTS plans_sub_renew_idx
    ON plans_subscriptions (status, current_period_end, next_retry_at);
