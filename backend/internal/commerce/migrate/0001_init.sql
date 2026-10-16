-- Depends on organization and ledger. No legacy business tables are used.
CREATE TABLE commerce_packages (
    id UUID PRIMARY KEY,
    operator_id UUID NOT NULL REFERENCES organization_operators(id),
    name TEXT NOT NULL CHECK(length(trim(name))>0),
    new_purchase_enabled BOOLEAN NOT NULL DEFAULT false,
    renewal_enabled BOOLEAN NOT NULL DEFAULT false,
    current_quote_id UUID,
    created_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE commerce_package_quotes (
    id UUID PRIMARY KEY,
    package_id UUID NOT NULL REFERENCES commerce_packages(id),
    operator_id UUID NOT NULL REFERENCES organization_operators(id),
    version BIGINT NOT NULL CHECK(version>0),
    price_usd NUMERIC(38,18) NOT NULL CHECK(price_usd>0),
    paid_atg NUMERIC(38,18) NOT NULL CHECK(paid_atg>0),
    gift_atg NUMERIC(38,18) NOT NULL CHECK(gift_atg>=0),
    period TEXT NOT NULL CHECK(period IN ('lifetime','month','quarter','year')),
    timezone TEXT NOT NULL,
    terms_reference TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE(package_id,version),
    UNIQUE(id,package_id)
);
ALTER TABLE commerce_packages ADD CONSTRAINT commerce_current_quote_fk FOREIGN KEY(current_quote_id,id) REFERENCES commerce_package_quotes(id,package_id) DEFERRABLE INITIALLY DEFERRED;
CREATE FUNCTION commerce_immutable_quote() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'sold quote versions are immutable' USING ERRCODE='23514'; END $$;
CREATE TRIGGER commerce_quote_immutable BEFORE UPDATE OR DELETE ON commerce_package_quotes FOR EACH ROW EXECUTE FUNCTION commerce_immutable_quote();
CREATE TABLE commerce_orders (
    id UUID PRIMARY KEY,
    operator_id UUID NOT NULL REFERENCES organization_operators(id),
    brand_id UUID NOT NULL REFERENCES organization_brands(id),
    customer_id UUID NOT NULL,
    ownership_version BIGINT NOT NULL CHECK(ownership_version>0),
    business_key TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    kind TEXT NOT NULL CHECK(kind IN ('package','renewal','topup')),
    quote_id UUID REFERENCES commerce_package_quotes(id),
    due_usd NUMERIC(38,18) NOT NULL CHECK(due_usd>0),
    paid_atg NUMERIC(38,18) NOT NULL CHECK(paid_atg>=0),
    package_gift_atg NUMERIC(38,18) NOT NULL CHECK(package_gift_atg>=0),
    merchant_gift_atg NUMERIC(38,18) NOT NULL CHECK(merchant_gift_atg>=0),
    kol_gift_atg NUMERIC(38,18) NOT NULL CHECK(kol_gift_atg>=0),
    internal_usd NUMERIC(38,18) NOT NULL CHECK(internal_usd>=0),
    commission_usd NUMERIC(38,18) NOT NULL CHECK(commission_usd>=0),
    external_allocated_usd NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK(external_allocated_usd>=0),
    hold_id UUID REFERENCES ledger_holds(id),
    internal_status TEXT NOT NULL CHECK(internal_status IN ('none','held','released','captured')),
    coverage_reference TEXT NOT NULL DEFAULT '',
    coverage_version BIGINT NOT NULL DEFAULT 0,
    coverage_status TEXT NOT NULL CHECK(coverage_status IN ('none','reserved','released','fulfilled')),
    marketing_reference TEXT NOT NULL DEFAULT '',
    commission_snapshot JSONB,
    status TEXT NOT NULL CHECK(status IN ('pending','partial','paid','completed','closed','exception')),
    exception_reason TEXT NOT NULL DEFAULT '',
    period TEXT NOT NULL CHECK(period IN ('lifetime','month','quarter','year')),
    timezone TEXT NOT NULL,
    subscription_id UUID,
    cycle_due_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ NOT NULL,
    closed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,
    FOREIGN KEY(customer_id,brand_id) REFERENCES organization_customers(id,brand_id),
    UNIQUE(operator_id,business_key),
    UNIQUE(subscription_id,cycle_due_at),
    CHECK(internal_usd+commission_usd<=due_usd),
    CHECK(external_allocated_usd<=due_usd-internal_usd-commission_usd),
    CHECK(kind<>'topup' OR (quote_id IS NULL AND paid_atg=0 AND package_gift_atg=0 AND merchant_gift_atg=0 AND kol_gift_atg=0 AND internal_usd=0 AND commission_usd=0))
);
CREATE INDEX commerce_customer_orders ON commerce_orders(operator_id,customer_id,created_at);
CREATE INDEX commerce_paid_delivery_queue ON commerce_orders(created_at,id) WHERE status IN ('paid','exception');
CREATE TABLE commerce_receipts (
    id UUID PRIMARY KEY,
    operator_id UUID NOT NULL REFERENCES organization_operators(id),
    provider TEXT NOT NULL,
    provider_receipt_id TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    gross_usd NUMERIC(38,18) NOT NULL CHECK(gross_usd>0),
    net_usd NUMERIC(38,18) CHECK(net_usd>=0),
    fee_usd NUMERIC(38,18) CHECK(fee_usd>=0),
    fee_status TEXT NOT NULL CHECK(fee_status IN ('pending','confirmed')),
    allocated_usd NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK(allocated_usd>=0 AND allocated_usd<=gross_usd),
    currency TEXT NOT NULL,
    original_amount TEXT NOT NULL,
    network TEXT NOT NULL DEFAULT '',
    recipient_reference TEXT NOT NULL,
    payer_reference TEXT NOT NULL,
    valuation_evidence TEXT NOT NULL,
    verification_evidence TEXT NOT NULL,
    verified_by TEXT NOT NULL,
    paid_at TIMESTAMPTZ,
    observed_at TIMESTAMPTZ NOT NULL,
    settled_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE(operator_id,provider,provider_receipt_id),
    CHECK((fee_status='pending' AND net_usd IS NULL AND fee_usd IS NULL AND settled_at IS NULL) OR (fee_status='confirmed' AND net_usd IS NOT NULL AND fee_usd IS NOT NULL AND net_usd+fee_usd=gross_usd))
);
CREATE TABLE commerce_receipt_allocations (
    id UUID PRIMARY KEY,
    operator_id UUID NOT NULL REFERENCES organization_operators(id),
    business_key TEXT NOT NULL,
    receipt_id UUID NOT NULL REFERENCES commerce_receipts(id),
    order_id UUID NOT NULL REFERENCES commerce_orders(id),
    amount_usd NUMERIC(38,18) NOT NULL CHECK(amount_usd>0),
    timing_proof TEXT NOT NULL CHECK(timing_proof IN ('actual_provider_time','observed_before_expiry')),
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE(operator_id,business_key)
);
CREATE FUNCTION commerce_allocate_receipt() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE r commerce_receipts; o commerce_orders;
BEGIN
    SELECT * INTO r FROM commerce_receipts WHERE id=NEW.receipt_id FOR UPDATE;
    SELECT * INTO o FROM commerce_orders WHERE id=NEW.order_id FOR UPDATE;
    IF r.operator_id<>NEW.operator_id OR o.operator_id<>NEW.operator_id THEN RAISE EXCEPTION 'receipt operating scope mismatch' USING ERRCODE='23514'; END IF;
    IF (r.paid_at IS NOT NULL AND r.paid_at>o.expires_at) OR (r.paid_at IS NULL AND r.observed_at>o.expires_at) THEN RAISE EXCEPTION 'timely payment cannot be proven' USING ERRCODE='23514'; END IF;
    IF (r.paid_at IS NULL AND NEW.timing_proof<>'observed_before_expiry') OR (r.paid_at IS NOT NULL AND NEW.timing_proof<>'actual_provider_time') THEN RAISE EXCEPTION 'payment time proof mismatch' USING ERRCODE='23514'; END IF;
    IF o.status='completed' THEN RAISE EXCEPTION 'order is already delivered' USING ERRCODE='23514'; END IF;
    UPDATE commerce_receipts SET allocated_usd=allocated_usd+NEW.amount_usd WHERE id=r.id;
    UPDATE commerce_orders SET external_allocated_usd=external_allocated_usd+NEW.amount_usd WHERE id=o.id;
    RETURN NEW;
END $$;
CREATE TRIGGER commerce_receipt_allocation_bounds BEFORE INSERT ON commerce_receipt_allocations FOR EACH ROW EXECUTE FUNCTION commerce_allocate_receipt();
CREATE FUNCTION commerce_immutable_allocation() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'receipt allocation requires explicit audited reversal' USING ERRCODE='23514'; END $$;
CREATE TRIGGER commerce_allocation_immutable BEFORE UPDATE OR DELETE ON commerce_receipt_allocations FOR EACH ROW EXECUTE FUNCTION commerce_immutable_allocation();
CREATE TABLE commerce_receipt_funds_reservations (
    id UUID PRIMARY KEY,
    operator_id UUID NOT NULL REFERENCES organization_operators(id),
    receipt_id UUID NOT NULL REFERENCES commerce_receipts(id),
    destination_id UUID NOT NULL,
    business_key TEXT NOT NULL,
    amount_usd NUMERIC(38,18) NOT NULL CHECK(amount_usd>0),
    deadline TIMESTAMPTZ NOT NULL,
    timing_proof TEXT NOT NULL CHECK(timing_proof IN ('actual_provider_time','observed_before_expiry')),
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE(operator_id,business_key)
);
CREATE FUNCTION commerce_reserve_receipt_funds() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE r commerce_receipts;
BEGIN
    SELECT * INTO r FROM commerce_receipts WHERE id=NEW.receipt_id FOR UPDATE;
    IF r.operator_id<>NEW.operator_id THEN RAISE EXCEPTION 'receipt operating scope mismatch' USING ERRCODE='23514'; END IF;
    IF (r.paid_at IS NOT NULL AND r.paid_at>NEW.deadline) OR (r.paid_at IS NULL AND r.observed_at>NEW.deadline) THEN RAISE EXCEPTION 'timely payment cannot be proven' USING ERRCODE='23514'; END IF;
    UPDATE commerce_receipts SET allocated_usd=allocated_usd+NEW.amount_usd WHERE id=r.id;
    RETURN NEW;
END $$;
CREATE TRIGGER commerce_receipt_funds_bounds BEFORE INSERT ON commerce_receipt_funds_reservations FOR EACH ROW EXECUTE FUNCTION commerce_reserve_receipt_funds();
CREATE TRIGGER commerce_receipt_funds_immutable BEFORE UPDATE OR DELETE ON commerce_receipt_funds_reservations FOR EACH ROW EXECUTE FUNCTION commerce_immutable_allocation();
CREATE TABLE commerce_deliveries (
    id UUID PRIMARY KEY,
    order_id UUID NOT NULL UNIQUE REFERENCES commerce_orders(id),
    grant_id UUID REFERENCES ledger_grants(id),
    effective_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ,
    delivered_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE commerce_subscriptions (
    id UUID PRIMARY KEY,
    operator_id UUID NOT NULL REFERENCES organization_operators(id),
    customer_id UUID NOT NULL REFERENCES organization_customers(id),
    package_id UUID NOT NULL REFERENCES commerce_packages(id),
    source_order_id UUID NOT NULL UNIQUE REFERENCES commerce_orders(id),
    authorized_quote_id UUID NOT NULL REFERENCES commerce_package_quotes(id),
    authorization_evidence TEXT NOT NULL,
    allow_commission BOOLEAN NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('active','cancelled','needs_confirmation','needs_funds')),
    next_due_at TIMESTAMPTZ NOT NULL,
    notice_quote_id UUID REFERENCES commerce_package_quotes(id),
    notice_cycle_due_at TIMESTAMPTZ,
    notice_delivered_at TIMESTAMPTZ,
    notice_evidence TEXT NOT NULL DEFAULT '',
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL
);
ALTER TABLE commerce_orders ADD CONSTRAINT commerce_order_subscription_fk FOREIGN KEY(subscription_id) REFERENCES commerce_subscriptions(id);
CREATE TABLE commerce_events (
    id UUID PRIMARY KEY,
    operator_id UUID NOT NULL REFERENCES organization_operators(id),
    aggregate_id UUID NOT NULL,
    kind TEXT NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    published_at TIMESTAMPTZ
);
