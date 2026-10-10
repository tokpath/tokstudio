-- Completed platform service-credit sales: actual receipt and quota delivery
-- share a single transaction. Administrative grants are not sales.
CREATE TABLE billing_oem_purchases (
 id TEXT PRIMARY KEY,
 operation_id TEXT NOT NULL UNIQUE,
 actor_user_id TEXT NOT NULL,
 oem_channel_org_id TEXT NOT NULL,
 cash_amount_minor BIGINT NOT NULL CHECK (cash_amount_minor > 0),
 cash_currency TEXT NOT NULL CHECK (cash_currency IN ('USD','CNY')),
 sale_amount_minor BIGINT NOT NULL CHECK (sale_amount_minor > 0),
 quota_amount_minor BIGINT NOT NULL CHECK (quota_amount_minor > 0),
 occurred_at TIMESTAMPTZ NOT NULL,
 completed_at TIMESTAMPTZ NOT NULL,
 external_reference TEXT NOT NULL DEFAULT '',
 note TEXT NOT NULL DEFAULT '',
 result_json JSONB NOT NULL
);
CREATE UNIQUE INDEX billing_oem_purchase_external_unique ON billing_oem_purchases(external_reference) WHERE external_reference <> '';
CREATE INDEX billing_oem_purchase_book ON billing_oem_purchases(oem_channel_org_id,occurred_at DESC,id DESC);
