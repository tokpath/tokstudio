CREATE TABLE cooperation_purchase_quotes (
 id UUID PRIMARY KEY,
 supplier_id UUID NOT NULL,
 buyer_id UUID NOT NULL,
 price_usd NUMERIC(38,18) NOT NULL CHECK(price_usd>0),
 paid_atg NUMERIC(38,18) NOT NULL CHECK(paid_atg>0),
 gift_atg NUMERIC(38,18) NOT NULL CHECK(gift_atg>=0),
 exit_terms JSONB NOT NULL,
 terms_reference TEXT NOT NULL,
 valid_until TIMESTAMPTZ,
 enabled BOOLEAN NOT NULL DEFAULT true,
 created_by UUID NOT NULL,
 created_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE cooperation_purchase_orders (
 id UUID PRIMARY KEY,
 supplier_id UUID NOT NULL,
 buyer_id UUID NOT NULL,
 quote_id UUID NOT NULL REFERENCES cooperation_purchase_quotes(id),
 business_key TEXT NOT NULL,
 fingerprint TEXT NOT NULL,
 due_usd NUMERIC(38,18) NOT NULL CHECK(due_usd>0),
 allocated_usd NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK(allocated_usd>=0 AND allocated_usd<=due_usd),
 paid_atg NUMERIC(38,18) NOT NULL,
 gift_atg NUMERIC(38,18) NOT NULL,
 exit_terms JSONB NOT NULL,
 terms_reference TEXT NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('awaiting_payment','paid','completed','closed','exception')),
 expires_at TIMESTAMPTZ NOT NULL,
 created_at TIMESTAMPTZ NOT NULL,
 completed_at TIMESTAMPTZ,
 UNIQUE(buyer_id,business_key)
);
CREATE TABLE cooperation_purchase_allocations (
 id UUID PRIMARY KEY,
 order_id UUID NOT NULL REFERENCES cooperation_purchase_orders(id),
 receipt_reservation_id UUID NOT NULL UNIQUE,
 receipt_id UUID NOT NULL,
 business_key TEXT NOT NULL,
 fingerprint TEXT NOT NULL,
 amount_usd NUMERIC(38,18) NOT NULL CHECK(amount_usd>0),
 created_at TIMESTAMPTZ NOT NULL,
 UNIQUE(order_id,business_key)
);
