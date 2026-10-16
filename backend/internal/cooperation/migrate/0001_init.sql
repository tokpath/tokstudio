CREATE TABLE cooperation_procurement_positions (
 buyer_id UUID PRIMARY KEY,
 supplier_id UUID NOT NULL,
 account_id UUID NOT NULL UNIQUE REFERENCES ledger_accounts(id),
 quantity NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK(quantity>=0),
 cost NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK(cost>=0),
 version BIGINT NOT NULL DEFAULT 0,
 CHECK(buyer_id<>supplier_id)
);
CREATE TABLE cooperation_procurement_lots (
 id UUID PRIMARY KEY,
 buyer_id UUID NOT NULL REFERENCES cooperation_procurement_positions(buyer_id),
 order_id TEXT NOT NULL UNIQUE,
 receipt_reference TEXT NOT NULL,
 paid_usd NUMERIC(38,18) NOT NULL CHECK(paid_usd>=0),
 paid_atg NUMERIC(38,18) NOT NULL CHECK(paid_atg>=0),
 gift_atg NUMERIC(38,18) NOT NULL CHECK(gift_atg>=0),
 remaining NUMERIC(38,18) NOT NULL CHECK(remaining>=0),
 paid_consumed NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK(paid_consumed>=0 AND paid_consumed<=paid_atg),
 gift_consumed NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK(gift_consumed>=0 AND gift_consumed<=gift_atg),
 fingerprint TEXT NOT NULL,
 exit_terms JSONB NOT NULL,
 delivered_at TIMESTAMPTZ NOT NULL,
 CHECK(paid_atg+gift_atg>0 AND remaining=paid_atg+gift_atg-paid_consumed-gift_consumed),
 CHECK(paid_usd=0 OR paid_atg>0)
);
CREATE TABLE cooperation_procurement_settlements (
 request_id TEXT NOT NULL,
 buyer_id UUID NOT NULL REFERENCES cooperation_procurement_positions(buyer_id),
 hold_id UUID NOT NULL UNIQUE REFERENCES ledger_holds(id),
 actual NUMERIC(38,18) NOT NULL CHECK(actual>=0),
 deducted NUMERIC(38,18) NOT NULL CHECK(deducted>=0),
 platform_shortfall NUMERIC(38,18) NOT NULL CHECK(platform_shortfall>=0),
 cost_usd NUMERIC(38,18) NOT NULL CHECK(cost_usd>=0),
 source_allocations JSONB NOT NULL,
 created_at TIMESTAMPTZ NOT NULL,
 PRIMARY KEY(request_id,buyer_id),
 CHECK(actual=deducted+platform_shortfall)
);
CREATE TABLE cooperation_oversell_policies (
 operator_id UUID PRIMARY KEY,
 allowance NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK(allowance>=0),
 warning_buffer NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK(warning_buffer>=0),
 version BIGINT NOT NULL DEFAULT 1,
 updated_by UUID NOT NULL,
 updated_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE cooperation_sales_commitments (
 id UUID PRIMARY KEY,
 operator_id UUID NOT NULL,
 order_id UUID NOT NULL UNIQUE,
 buyer_id UUID,
 atg NUMERIC(38,18) NOT NULL CHECK(atg>=0),
 status TEXT NOT NULL CHECK(status IN ('active','fulfilled','released')),
 catalog_version BIGINT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL,
 updated_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX cooperation_commitments_active ON cooperation_sales_commitments(operator_id) WHERE status='active';
