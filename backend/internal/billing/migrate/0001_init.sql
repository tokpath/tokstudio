-- billing 模块自有表。其他模块禁止直连这些表或复用内部 ORM Model。
-- 金额一律使用最小货币单位（1 USD = 1_000_000 micro-USD），禁止浮点列。

CREATE TABLE IF NOT EXISTS billing_wallets (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    currency TEXT NOT NULL DEFAULT 'USD',
    available_minor BIGINT NOT NULL DEFAULT 0,
    reserved_minor BIGINT NOT NULL DEFAULT 0,
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, currency),
    CONSTRAINT billing_wallets_nonneg CHECK (available_minor >= 0 AND reserved_minor >= 0)
);

CREATE TABLE IF NOT EXISTS billing_ledger (
    id TEXT PRIMARY KEY,
    wallet_id TEXT NOT NULL REFERENCES billing_wallets(id),
    event_type TEXT NOT NULL,
    amount_minor BIGINT NOT NULL,
    currency TEXT NOT NULL DEFAULT 'USD',
    balance_after_minor BIGINT NOT NULL,
    reserved_after_minor BIGINT NOT NULL,
    reference_type TEXT,
    reference_id TEXT,
    idempotency_key TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS billing_quota_accounts (
    id TEXT PRIMARY KEY,
    owner_type TEXT NOT NULL,
    owner_id TEXT NOT NULL,
    unit_type TEXT NOT NULL DEFAULT 'usd_credit',
    available_minor BIGINT NOT NULL DEFAULT 0,
    reserved_minor BIGINT NOT NULL DEFAULT 0,
    version BIGINT NOT NULL DEFAULT 1,
    UNIQUE (owner_type, owner_id, unit_type),
    CONSTRAINT billing_quota_nonneg CHECK (available_minor >= 0 AND reserved_minor >= 0)
);

CREATE TABLE IF NOT EXISTS billing_quota_ledger (
    id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES billing_quota_accounts(id),
    event_type TEXT NOT NULL,
    amount_minor BIGINT NOT NULL,
    reference_type TEXT,
    reference_id TEXT,
    idempotency_key TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS billing_topups (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    channel_org_id TEXT,
    amount_minor BIGINT NOT NULL,
    currency TEXT NOT NULL DEFAULT 'USD',
    payment_method TEXT NOT NULL,
    status TEXT NOT NULL,
    provider_trade_id TEXT,
    redeem_code TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS billing_redeem_codes (
    id TEXT PRIMARY KEY,
    code TEXT NOT NULL UNIQUE,
    amount_minor BIGINT NOT NULL,
    currency TEXT NOT NULL DEFAULT 'USD',
    max_redemptions INTEGER NOT NULL DEFAULT 1,
    redeemed_count INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'active'
);

CREATE TABLE IF NOT EXISTS billing_authorizations (
    id TEXT PRIMARY KEY,
    wallet_id TEXT NOT NULL REFERENCES billing_wallets(id),
    user_id TEXT NOT NULL,
    channel_org_id TEXT,
    request_id TEXT NOT NULL UNIQUE,
    amount_minor BIGINT NOT NULL,
    settled_minor BIGINT NOT NULL DEFAULT 0,
    currency TEXT NOT NULL DEFAULT 'USD',
    status TEXT NOT NULL,
    price_version_id TEXT,
    unit_prices_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS billing_usage_events (
    id TEXT PRIMARY KEY,
    request_id TEXT NOT NULL,
    attempt_id TEXT,
    user_id TEXT NOT NULL,
    api_key_id TEXT,
    channel_org_id TEXT,
    public_model_id TEXT NOT NULL,
    provider_id TEXT,
    upstream_model_id TEXT,
    unit_usage_json JSONB NOT NULL,
    unit_prices_json JSONB NOT NULL,
    price_version_id TEXT,
    customer_amount_minor BIGINT NOT NULL,
    upstream_cost_minor BIGINT NOT NULL,
    wholesale_amount_minor BIGINT NOT NULL,
    currency TEXT NOT NULL DEFAULT 'USD',
    state TEXT NOT NULL,
    idempotency_key TEXT NOT NULL UNIQUE,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS billing_customer_charges (
    id TEXT PRIMARY KEY,
    request_id TEXT NOT NULL UNIQUE,
    usage_event_id TEXT NOT NULL REFERENCES billing_usage_events(id),
    authorization_id TEXT REFERENCES billing_authorizations(id),
    amount_minor BIGINT NOT NULL,
    price_version_id TEXT,
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS billing_cost_entries (
    id TEXT PRIMARY KEY,
    request_id TEXT NOT NULL,
    attempt_id TEXT NOT NULL UNIQUE,
    provider_id TEXT NOT NULL,
    amount_minor BIGINT NOT NULL,
    currency TEXT NOT NULL DEFAULT 'USD',
    unit_usage_json JSONB NOT NULL,
    unit_prices_json JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS billing_commission_ledger (
    id TEXT PRIMARY KEY,
    usage_event_id TEXT NOT NULL,
    channel_org_id TEXT,
    policy_version TEXT NOT NULL DEFAULT 'm3-stub-v1',
    base_amount_minor BIGINT NOT NULL,
    amount_minor BIGINT NOT NULL,
    status TEXT NOT NULL,
    reversal_of TEXT,
    idempotency_key TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS billing_ledger_wallet_idx ON billing_ledger (wallet_id, created_at DESC);
CREATE INDEX IF NOT EXISTS billing_usage_user_idx ON billing_usage_events (user_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS billing_auth_expiry_idx ON billing_authorizations (status, expires_at);
CREATE INDEX IF NOT EXISTS billing_topup_user_idx ON billing_topups (user_id, created_at DESC);
