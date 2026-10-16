CREATE TABLE usage_keys (
 id UUID PRIMARY KEY,
 customer_id UUID NOT NULL,
 brand_id UUID NOT NULL,
 name TEXT NOT NULL,
 kind TEXT NOT NULL DEFAULT 'api' CHECK(kind IN ('api','playground')),
 prefix TEXT NOT NULL,
 token_hash TEXT NOT NULL UNIQUE,
 status TEXT NOT NULL CHECK(status IN ('active','disabled','deleted')),
 model_ids JSONB,
 budget_limit NUMERIC(38,18) CHECK(budget_limit>=0),
 period TEXT NOT NULL CHECK(period IN ('lifetime','month','quarter','year')),
 timezone TEXT NOT NULL,
 rate_limit_rpm BIGINT CHECK(rate_limit_rpm>0),
 concurrency_limit BIGINT CHECK(concurrency_limit>0),
 version BIGINT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL,
 updated_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX usage_keys_customer ON usage_keys(customer_id,created_at);
CREATE UNIQUE INDEX usage_one_playground ON usage_keys(customer_id) WHERE kind='playground';
CREATE TABLE usage_key_commands (
 customer_id UUID NOT NULL,
 business_key TEXT NOT NULL,
 key_id UUID NOT NULL REFERENCES usage_keys(id),
 action TEXT NOT NULL,
 fingerprint TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL,
 PRIMARY KEY(customer_id,business_key)
);
CREATE TABLE usage_requests (
 id UUID PRIMARY KEY,
 customer_id UUID NOT NULL,
 operator_id UUID NOT NULL,
 brand_id UUID NOT NULL,
 ownership_version BIGINT NOT NULL,
 key_id UUID NOT NULL REFERENCES usage_keys(id),
 business_key TEXT NOT NULL,
 fingerprint TEXT NOT NULL,
 model_id TEXT NOT NULL,
 operation TEXT NOT NULL,
 streaming BOOLEAN NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('accepted','executing','submitted','awaiting_usage','completed','failed','free','cancelled')),
 accepted_at TIMESTAMPTZ NOT NULL,
 usage_deadline TIMESTAMPTZ NOT NULL,
 window_start TIMESTAMPTZ NOT NULL,
 window_end TIMESTAMPTZ,
 budget_limit NUMERIC(38,18),
 cancel_requested BOOLEAN NOT NULL DEFAULT false,
 estimate_atg NUMERIC(38,18) NOT NULL CHECK(estimate_atg>=0),
 actual_atg NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK(actual_atg>=0),
 charged_atg NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK(charged_atg>=0),
 merchant_shortfall NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK(merchant_shortfall>=0),
 budget_overage NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK(budget_overage>=0),
 customer_hold_id UUID REFERENCES ledger_holds(id),
 supply_holds JSONB NOT NULL,
 price_snapshot JSONB NOT NULL,
 candidates JSONB NOT NULL,
 settlement JSONB,
 reason TEXT NOT NULL DEFAULT '',
 completed_at TIMESTAMPTZ,
 UNIQUE(key_id,business_key)
);
CREATE INDEX usage_requests_budget ON usage_requests(key_id,accepted_at);
CREATE INDEX usage_requests_pending ON usage_requests(usage_deadline) WHERE status IN ('accepted','executing','submitted','awaiting_usage');
CREATE INDEX usage_requests_customer ON usage_requests(customer_id,accepted_at DESC);
CREATE TABLE usage_attempts (
 id UUID PRIMARY KEY,
 request_id UUID NOT NULL REFERENCES usage_requests(id),
 upstream_id TEXT NOT NULL,
 provider_id TEXT NOT NULL,
 definition JSONB NOT NULL,
 state TEXT NOT NULL,
 provider_request_id TEXT NOT NULL DEFAULT '',
 task_id TEXT NOT NULL DEFAULT '',
 usage JSONB,
 supplier_cost_usd NUMERIC(38,18) NOT NULL DEFAULT 0,
 cost_status TEXT NOT NULL DEFAULT 'pending',
 cost_recorded_at TIMESTAMPTZ,
 error_code TEXT NOT NULL DEFAULT '',
 output_delivered BOOLEAN NOT NULL DEFAULT false,
 started_at TIMESTAMPTZ NOT NULL,
 completed_at TIMESTAMPTZ
);
CREATE TABLE usage_results (
 request_id UUID PRIMARY KEY REFERENCES usage_requests(id),
 content_type TEXT NOT NULL,
 object_key TEXT NOT NULL UNIQUE,
 digest TEXT NOT NULL,
 state TEXT NOT NULL CHECK(state IN ('pending','ready')),
 expires_at TIMESTAMPTZ NOT NULL,
 created_at TIMESTAMPTZ NOT NULL
);
