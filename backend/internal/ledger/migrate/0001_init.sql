CREATE TABLE ledger_accounts (
  id UUID PRIMARY KEY,
  operator_id UUID NOT NULL,
  owner_id UUID NOT NULL,
  purpose TEXT NOT NULL,
  unit TEXT NOT NULL CHECK (unit IN ('USD','ATG')),
  allow_negative BOOLEAN NOT NULL DEFAULT FALSE,
  balance NUMERIC(38,18) NOT NULL DEFAULT 0,
  held NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK (held >= 0),
  version BIGINT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(operator_id,owner_id,purpose,unit),
  CHECK ((allow_negative AND purpose LIKE 'control:%') OR (NOT allow_negative AND balance >= held)),
  CHECK (NOT allow_negative OR held = 0)
);
CREATE TABLE ledger_journals (
  id UUID PRIMARY KEY,
  operator_id UUID NOT NULL,
  business_key TEXT NOT NULL,
  kind TEXT NOT NULL,
  source_id TEXT NOT NULL,
  fingerprint TEXT NOT NULL,
  actor_id TEXT NOT NULL,
  reason TEXT NOT NULL,
  original_id UUID REFERENCES ledger_journals(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(operator_id,business_key)
);
CREATE TABLE ledger_entries (
  id UUID PRIMARY KEY,
  journal_id UUID NOT NULL REFERENCES ledger_journals(id),
  account_id UUID NOT NULL REFERENCES ledger_accounts(id),
  unit TEXT NOT NULL CHECK (unit IN ('USD','ATG')),
  amount NUMERIC(38,18) NOT NULL CHECK (amount <> 0),
  balance_after NUMERIC(38,18) NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(journal_id,account_id)
);
CREATE INDEX ledger_entries_account_time ON ledger_entries(account_id,created_at,id);
CREATE INDEX ledger_journals_operator_time ON ledger_journals(operator_id,created_at,id);
CREATE TABLE ledger_holds (
  id UUID PRIMARY KEY,
  operator_id UUID NOT NULL,
  business_key TEXT NOT NULL,
  fingerprint TEXT NOT NULL,
  purpose TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('active','released','captured')),
  expires_at TIMESTAMPTZ NOT NULL,
  resolution_key TEXT,
  journal_id UUID REFERENCES ledger_journals(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  resolved_at TIMESTAMPTZ,
  UNIQUE(operator_id,business_key)
);
CREATE TABLE ledger_hold_allocations (
  hold_id UUID NOT NULL REFERENCES ledger_holds(id),
  account_id UUID NOT NULL REFERENCES ledger_accounts(id),
  amount NUMERIC(38,18) NOT NULL CHECK (amount > 0),
  PRIMARY KEY(hold_id,account_id)
);
CREATE INDEX ledger_holds_active_expiry ON ledger_holds(expires_at,id) WHERE status='active';
CREATE TABLE ledger_carries (
  account_id UUID NOT NULL REFERENCES ledger_accounts(id),
  source_key TEXT NOT NULL,
  numerator TEXT NOT NULL,
  denominator TEXT NOT NULL,
  version BIGINT NOT NULL DEFAULT 0,
  PRIMARY KEY(account_id,source_key)
);

CREATE FUNCTION ledger_immutable_fact() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'ledger facts are append-only'; END $$;
CREATE TRIGGER ledger_journals_immutable BEFORE UPDATE OR DELETE ON ledger_journals FOR EACH ROW EXECUTE FUNCTION ledger_immutable_fact();
CREATE TRIGGER ledger_entries_immutable BEFORE UPDATE OR DELETE ON ledger_entries FOR EACH ROW EXECUTE FUNCTION ledger_immutable_fact();

CREATE FUNCTION ledger_validate_journal() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM ledger_entries WHERE journal_id=NEW.id) THEN
    RAISE EXCEPTION 'journal has no entries';
  END IF;
  IF EXISTS (SELECT unit FROM ledger_entries WHERE journal_id=NEW.id GROUP BY unit HAVING sum(amount)<>0) THEN
    RAISE EXCEPTION 'journal is not balanced per unit';
  END IF;
  IF EXISTS (SELECT 1 FROM ledger_entries e JOIN ledger_accounts a ON a.id=e.account_id WHERE e.journal_id=NEW.id AND (e.unit<>a.unit OR a.operator_id<>NEW.operator_id)) THEN
    RAISE EXCEPTION 'journal account scope or unit mismatch';
  END IF;
  RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER ledger_journals_balanced AFTER INSERT ON ledger_journals DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION ledger_validate_journal();
