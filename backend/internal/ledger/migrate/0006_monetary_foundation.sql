CREATE TABLE ledger_monetary_settings (
 id TEXT PRIMARY KEY CHECK(id='platform'), currency TEXT NOT NULL CHECK(currency ~ '^[A-Z]{3}$'),
 atg_per_unit NUMERIC(38,18) NOT NULL CHECK(atg_per_unit>0),
 effective_at TIMESTAMPTZ, version BIGINT NOT NULL DEFAULT 1,
 configured_by TEXT NOT NULL DEFAULT '', confirmed_at TIMESTAMPTZ
);
INSERT INTO ledger_monetary_settings(id,currency,atg_per_unit) VALUES('platform','USD',1);
CREATE TABLE ledger_monetary_events (
 id UUID PRIMARY KEY,actor_id TEXT NOT NULL,action TEXT NOT NULL CHECK(action IN ('scheduled','cancelled')),
 currency TEXT NOT NULL,atg_per_unit NUMERIC(38,18) NOT NULL,effective_at TIMESTAMPTZ NOT NULL,
 version BIGINT NOT NULL,created_at TIMESTAMPTZ NOT NULL
);
CREATE FUNCTION ledger_monetary_lock() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'monetary settings cannot be deleted'; END IF;
 IF OLD.effective_at IS NOT NULL AND OLD.effective_at<=clock_timestamp() THEN
   RAISE EXCEPTION 'monetary foundation is permanently locked';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER ledger_monetary_locked BEFORE UPDATE OR DELETE ON ledger_monetary_settings FOR EACH ROW EXECUTE FUNCTION ledger_monetary_lock();
CREATE TRIGGER ledger_monetary_events_immutable BEFORE UPDATE OR DELETE ON ledger_monetary_events FOR EACH ROW EXECUTE FUNCTION ledger_immutable_fact();

-- MONEY is the immutable system settlement unit; external ISO currencies stay
-- distinct. This is a schema unit rename, with no amount or balance conversion.
ALTER TABLE ledger_accounts DROP CONSTRAINT ledger_accounts_unit_check;
ALTER TABLE ledger_entries DROP CONSTRAINT ledger_entries_unit_check;
ALTER TABLE ledger_entries DISABLE TRIGGER ledger_entries_immutable;
UPDATE ledger_accounts SET unit='MONEY' WHERE unit='USD';
UPDATE ledger_entries SET unit='MONEY' WHERE unit='USD';
ALTER TABLE ledger_entries ENABLE TRIGGER ledger_entries_immutable;
ALTER TABLE ledger_accounts ADD CONSTRAINT ledger_accounts_unit_check CHECK(unit IN ('MONEY','ATG'));
ALTER TABLE ledger_entries ADD CONSTRAINT ledger_entries_unit_check CHECK(unit IN ('MONEY','ATG'));
