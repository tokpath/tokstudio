-- Provenance projections never manufacture cash; ledger remains the balance authority.
CREATE TABLE commerce_wallet_funding_lots (
    id UUID PRIMARY KEY,
    operator_id UUID NOT NULL REFERENCES organization_operators(id),
    source_operator_id UUID NOT NULL REFERENCES organization_operators(id),
    customer_id UUID NOT NULL REFERENCES organization_customers(id),
    top_up_order_id UUID NOT NULL REFERENCES commerce_orders(id),
    receipt_id UUID NOT NULL REFERENCES commerce_receipts(id),
    receipt_allocation_id UUID NOT NULL UNIQUE REFERENCES commerce_receipt_allocations(id),
    amount_usd NUMERIC(38,18) NOT NULL CHECK(amount_usd>0),
    reserved_usd NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK(reserved_usd>=0),
    spent_usd NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK(spent_usd>=0),
    created_at TIMESTAMPTZ NOT NULL,
    CHECK(reserved_usd+spent_usd<=amount_usd)
);
CREATE INDEX commerce_wallet_funding_available ON commerce_wallet_funding_lots(operator_id,customer_id,created_at,id);
CREATE TABLE commerce_wallet_funding_commands (
    id UUID PRIMARY KEY,
    operator_id UUID NOT NULL REFERENCES organization_operators(id),
    customer_id UUID NOT NULL REFERENCES organization_customers(id),
    kind TEXT NOT NULL CHECK(kind IN ('order','refund','restore')),
    reference_id UUID NOT NULL,
    source_order_id UUID REFERENCES commerce_orders(id),
    restricted_receipt_id UUID REFERENCES commerce_receipts(id),
    attempt INT NOT NULL CHECK(attempt>0),
    amount_usd NUMERIC(38,18) NOT NULL CHECK(amount_usd>0),
    status TEXT NOT NULL CHECK(status IN ('reserved','consumed','released','partial','restored')),
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE(kind,reference_id,attempt)
);
CREATE TABLE commerce_wallet_funding_allocations (
    id UUID PRIMARY KEY,
    command_id UUID NOT NULL REFERENCES commerce_wallet_funding_commands(id),
    lot_id UUID NOT NULL REFERENCES commerce_wallet_funding_lots(id),
    amount_usd NUMERIC(38,18) NOT NULL CHECK(amount_usd>0),
    restored_usd NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK(restored_usd>=0 AND restored_usd<=amount_usd),
    status TEXT NOT NULL CHECK(status IN ('reserved','consumed','released','restored')),
    original_allocation_id UUID REFERENCES commerce_wallet_funding_allocations(id),
    UNIQUE(command_id,lot_id)
);
CREATE INDEX commerce_wallet_funding_command_allocations ON commerce_wallet_funding_allocations(command_id,id);
