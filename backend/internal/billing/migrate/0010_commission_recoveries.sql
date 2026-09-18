-- Paid commissions reversed by a customer refund require manual reconciliation.
-- This is a receivable record, never an automatic debit of API or future commission funds.
CREATE TABLE IF NOT EXISTS billing_commission_recoveries (
    id TEXT PRIMARY KEY,
    commission_entry_id TEXT NOT NULL UNIQUE,
    wallet_id TEXT NOT NULL REFERENCES billing_wallets(id),
    settlement_id TEXT NOT NULL,
    credit_ledger_id TEXT NOT NULL REFERENCES billing_ledger(id),
    payout_ledger_id TEXT NOT NULL REFERENCES billing_ledger(id),
    amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),
    status TEXT NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS billing_commission_recoveries_wallet_idx
    ON billing_commission_recoveries(wallet_id, status);
