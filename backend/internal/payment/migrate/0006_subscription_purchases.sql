-- Durable purchase identity: one user operation creates one subscription and order.
CREATE TABLE IF NOT EXISTS payment_subscription_purchases (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    request_hash TEXT NOT NULL,
    subscription_id TEXT,
    order_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS payment_subscription_purchase_user_idx ON payment_subscription_purchases(user_id, created_at DESC);
