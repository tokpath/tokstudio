CREATE TABLE IF NOT EXISTS payment_wallet_purchases (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    request_hash TEXT NOT NULL,
    order_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS payment_wallet_purchase_user_idx ON payment_wallet_purchases(user_id, created_at DESC);
