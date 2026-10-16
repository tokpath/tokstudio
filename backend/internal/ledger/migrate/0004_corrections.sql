ALTER TABLE ledger_grants ADD COLUMN cancelled NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK(cancelled>=0);
ALTER TABLE ledger_grants ADD COLUMN cancelled_paid_usd NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK(cancelled_paid_usd>=0);
ALTER TABLE ledger_grants ADD CONSTRAINT ledger_grants_total_attribution CHECK(consumed+expired+cancelled<=total);
ALTER TABLE ledger_grants ADD CONSTRAINT ledger_grants_paid_attribution CHECK(consumed_paid_usd+expired_paid_usd+cancelled_paid_usd<=paid_usd);
ALTER TABLE ledger_grant_sources ADD COLUMN cancelled NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK(cancelled>=0);
ALTER TABLE ledger_grant_sources ADD CONSTRAINT ledger_sources_total_attribution CHECK(consumed+expired+cancelled<=quantity);
CREATE TABLE ledger_grant_reversals (
 id UUID PRIMARY KEY,
 operator_id UUID NOT NULL,
 original_event_id UUID NOT NULL REFERENCES ledger_grant_events(id),
 reversal_event_id UUID NOT NULL UNIQUE REFERENCES ledger_grant_events(id),
 business_key TEXT NOT NULL,
 fingerprint TEXT NOT NULL,
 amount NUMERIC(38,18) NOT NULL CHECK(amount>0),
 paid_usd NUMERIC(38,18) NOT NULL CHECK(paid_usd>=0),
 sources JSONB NOT NULL,
 created_at TIMESTAMPTZ NOT NULL,
 UNIQUE(operator_id,business_key)
);
CREATE TRIGGER ledger_grant_reversals_immutable BEFORE UPDATE OR DELETE ON ledger_grant_reversals FOR EACH ROW EXECUTE FUNCTION ledger_immutable_fact();
