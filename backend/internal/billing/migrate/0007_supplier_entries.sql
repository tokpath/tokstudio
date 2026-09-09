-- 供应商线下支出：真实付款在站外完成，这里只记账。金额存负数。
CREATE TABLE IF NOT EXISTS billing_supplier_entries (
    id TEXT PRIMARY KEY,
    channel_org_id TEXT NOT NULL,
    amount_minor BIGINT NOT NULL,
    currency TEXT NOT NULL DEFAULT 'USD',
    occurred_at TIMESTAMPTZ NOT NULL,
    source_type TEXT NOT NULL,
    source_id TEXT,
    provider_id TEXT,
    vendor_name TEXT,
    invoice_no TEXT,
    payment_method TEXT,
    bank_ref TEXT,
    counterparty TEXT,
    memo TEXT,
    attachment_url TEXT,
    actor_user_id TEXT NOT NULL,
    reversal_of TEXT,
    idempotency_key TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS billing_supplier_channel_idx
    ON billing_supplier_entries (channel_org_id, occurred_at DESC);
