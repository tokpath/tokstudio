-- payment 模块自有表。其他模块禁止直连。
CREATE TABLE IF NOT EXISTS payment_orders (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    adapter TEXT NOT NULL,
    purpose TEXT NOT NULL,
    reference_type TEXT,
    reference_id TEXT,
    amount_minor BIGINT NOT NULL,
    currency TEXT NOT NULL DEFAULT 'USD',
    status TEXT NOT NULL,
    provider_trade_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS payment_events (
    id TEXT PRIMARY KEY,
    adapter TEXT NOT NULL,
    external_event_id TEXT NOT NULL UNIQUE,
    order_id TEXT,
    signature_valid BOOLEAN NOT NULL,
    payload_json JSONB NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS payment_orders_user_idx ON payment_orders (user_id, created_at DESC);
