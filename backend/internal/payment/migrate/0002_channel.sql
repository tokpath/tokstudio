-- 支付单挂上渠道；凭证和收银台规则按 channel_org 隔离。
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS channel_org_id TEXT NOT NULL DEFAULT '';
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS credit_minor BIGINT NOT NULL DEFAULT 0;

UPDATE payment_orders
SET channel_org_id = 'chn_official_a'
WHERE channel_org_id IS NULL OR channel_org_id = '';

UPDATE payment_orders
SET credit_minor = amount_minor
WHERE credit_minor = 0;

CREATE INDEX IF NOT EXISTS payment_orders_channel_idx ON payment_orders (channel_org_id, created_at DESC);

CREATE TABLE IF NOT EXISTS payment_provider_instances (
    id TEXT PRIMARY KEY,
    channel_org_id TEXT NOT NULL,
    adapter TEXT NOT NULL,
    name TEXT NOT NULL,
    mode TEXT NOT NULL DEFAULT 'sandbox',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    refund_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order INTEGER NOT NULL DEFAULT 0,
    min_amount_minor BIGINT,
    max_amount_minor BIGINT,
    daily_limit_minor BIGINT,
    credentials_ciphertext TEXT NOT NULL DEFAULT '',
    credentials_meta JSONB NOT NULL DEFAULT '{}'::jsonb,
    last_tested_at TIMESTAMPTZ,
    last_test_ok BOOLEAN,
    last_paid_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS payment_instances_channel_idx
    ON payment_provider_instances (channel_org_id, adapter, sort_order);

CREATE TABLE IF NOT EXISTS payment_channel_settings (
    channel_org_id TEXT PRIMARY KEY,
    quick_amounts_json JSONB NOT NULL DEFAULT '[100, 300, 500, 1000]'::jsonb,
    min_pay_major BIGINT NOT NULL DEFAULT 10,
    max_pay_major BIGINT NOT NULL DEFAULT 50000,
    timeout_minutes INTEGER NOT NULL DEFAULT 15,
    help_text TEXT NOT NULL DEFAULT '',
    help_image_url TEXT NOT NULL DEFAULT '',
    product_prefix TEXT NOT NULL DEFAULT '',
    product_suffix TEXT NOT NULL DEFAULT '',
    max_pending_orders INTEGER NOT NULL DEFAULT 3,
    cancel_rate_limit INTEGER NOT NULL DEFAULT 10,
    fee_bps INTEGER NOT NULL DEFAULT 0,
    online_disabled BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS payment_adapter_flags (
    adapter TEXT PRIMARY KEY,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO payment_adapter_flags (adapter, enabled, updated_at) VALUES
    ('alipay', TRUE, now()),
    ('wechat', TRUE, now()),
    ('stripe', TRUE, now()),
    ('manual', TRUE, now())
ON CONFLICT (adapter) DO NOTHING;
