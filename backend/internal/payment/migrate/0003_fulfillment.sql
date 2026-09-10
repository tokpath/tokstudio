-- 已支付订单的履约时间。paid 但 fulfilled_at 为空时 webhook 重放与 Worker 可再履约。
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS fulfilled_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS payment_orders_unfulfilled_idx
    ON payment_orders (status, fulfilled_at)
    WHERE status = 'paid' AND fulfilled_at IS NULL;
