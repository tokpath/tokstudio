CREATE TABLE IF NOT EXISTS payments_merchants (
 id text PRIMARY KEY, operator_id text NOT NULL, driver text NOT NULL, definition jsonb NOT NULL,
 credentials_ciphertext text NOT NULL, enabled boolean NOT NULL, version bigint NOT NULL CHECK(version>0)
);
CREATE INDEX IF NOT EXISTS payments_merchants_operator ON payments_merchants(operator_id);
CREATE TABLE IF NOT EXISTS payments_merchant_versions (
 merchant_id text NOT NULL, version bigint NOT NULL, operator_id text NOT NULL,
 definition jsonb NOT NULL, credentials_ciphertext text NOT NULL, created_at timestamptz NOT NULL,
 PRIMARY KEY(merchant_id,version)
);
CREATE TABLE IF NOT EXISTS payments_intents (
 merchant_id text NOT NULL REFERENCES payments_merchants(id), order_id text NOT NULL,
 operator_id text NOT NULL, fingerprint text NOT NULL, state text NOT NULL,
 order_ciphertext text NOT NULL, checkout_ciphertext text, provider_id text,
 created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL,
 PRIMARY KEY(merchant_id,order_id)
);
CREATE TABLE IF NOT EXISTS payments_webhook_inbox (
 merchant_id text NOT NULL REFERENCES payments_merchants(id), event_id text NOT NULL,
 body_digest text NOT NULL, hint jsonb NOT NULL, state text NOT NULL,
 received_at timestamptz NOT NULL, processed_at timestamptz, error_code text,
 attempts integer NOT NULL DEFAULT 0, next_attempt_at timestamptz NOT NULL DEFAULT now(), lease_until timestamptz,
 PRIMARY KEY(merchant_id,event_id)
);
CREATE TABLE IF NOT EXISTS payments_verified_facts (
 id text PRIMARY KEY, merchant_id text NOT NULL REFERENCES payments_merchants(id),
 operator_id text NOT NULL, order_id text NOT NULL, kind text NOT NULL, fingerprint text NOT NULL,
 fact jsonb NOT NULL, valuation jsonb NOT NULL, receipt_id text, state text NOT NULL,
 observed_at timestamptz NOT NULL
);
CREATE TABLE IF NOT EXISTS payments_valuation_residuals (
 fact_id text PRIMARY KEY REFERENCES payments_verified_facts(id), operator_id text NOT NULL,
 order_id text NOT NULL, numerator text NOT NULL, denominator text NOT NULL,
 valuation_reference text NOT NULL, state text NOT NULL DEFAULT 'unposted'
);
CREATE TABLE IF NOT EXISTS payments_refund_intents (
 merchant_id text NOT NULL REFERENCES payments_merchants(id), refund_id text NOT NULL,
 operator_id text NOT NULL, order_id text NOT NULL, fingerprint text NOT NULL, state text NOT NULL,
 refund_ciphertext text NOT NULL, provider_id text, result_ciphertext text,
 created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL,
 PRIMARY KEY(merchant_id,refund_id)
);
