CREATE TABLE ledger_grants (
 id UUID PRIMARY KEY,
 operator_id UUID NOT NULL,
 customer_id UUID NOT NULL,
 account_id UUID NOT NULL UNIQUE REFERENCES ledger_accounts(id),
 order_id TEXT NOT NULL,
 business_key TEXT NOT NULL,
 fingerprint TEXT NOT NULL,
 paid_usd NUMERIC(38,18) NOT NULL CHECK(paid_usd>=0),
 total NUMERIC(38,18) NOT NULL CHECK(total>0),
 consumed NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK(consumed>=0),
 expired NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK(expired>=0),
 consumed_paid_usd NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK(consumed_paid_usd>=0),
 expired_paid_usd NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK(expired_paid_usd>=0),
 effective_at TIMESTAMPTZ NOT NULL,
 expires_at TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(operator_id,business_key),
 CHECK(expires_at IS NULL OR expires_at>effective_at),
 CHECK(consumed+expired<=total),
 CHECK(consumed_paid_usd+expired_paid_usd<=paid_usd)
);
CREATE INDEX ledger_grants_fefo ON ledger_grants(operator_id,customer_id,expires_at ASC NULLS LAST,effective_at,id);
CREATE TABLE ledger_grant_sources (
 grant_id UUID NOT NULL REFERENCES ledger_grants(id),
 kind TEXT NOT NULL,
 quantity NUMERIC(38,18) NOT NULL CHECK(quantity>0),
 consumed NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK(consumed>=0),
 expired NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK(expired>=0),
 PRIMARY KEY(grant_id,kind),
 CHECK(consumed+expired<=quantity)
);
CREATE TABLE ledger_credit_results (
 hold_id UUID PRIMARY KEY REFERENCES ledger_holds(id),
 fingerprint TEXT NOT NULL,
 result JSONB NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE ledger_grant_events (
 id UUID PRIMARY KEY,
 grant_id UUID NOT NULL REFERENCES ledger_grants(id),
 business_key TEXT NOT NULL,
 kind TEXT NOT NULL CHECK(kind IN ('usage','expiry','reversal','cancellation')),
 amount NUMERIC(38,18) NOT NULL,
 paid_usd NUMERIC(38,18) NOT NULL,
 sources JSONB NOT NULL,
 journal_id UUID REFERENCES ledger_journals(id),
 created_at TIMESTAMPTZ NOT NULL,
 UNIQUE(grant_id,business_key)
);
CREATE INDEX ledger_grant_events_pending_confirm ON ledger_grant_events(created_at,id);
CREATE TRIGGER ledger_grant_events_immutable BEFORE UPDATE OR DELETE ON ledger_grant_events FOR EACH ROW EXECUTE FUNCTION ledger_immutable_fact();
