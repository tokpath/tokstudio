CREATE TABLE incentives_commission_funding_lots (
 entry_id uuid PRIMARY KEY REFERENCES incentives_commission_entries(id), operator_id uuid NOT NULL, original_operator_id uuid NOT NULL,
 customer_id uuid NOT NULL, amount_usd numeric(38,18) NOT NULL CHECK(amount_usd>0),
 reserved_usd numeric(38,18) NOT NULL DEFAULT 0 CHECK(reserved_usd>=0), consumed_usd numeric(38,18) NOT NULL DEFAULT 0 CHECK(consumed_usd>=0),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), CHECK(reserved_usd+consumed_usd<=amount_usd)
);
CREATE TABLE incentives_commission_funding_commands (
 id uuid PRIMARY KEY, operator_id uuid NOT NULL, customer_id uuid NOT NULL, kind text NOT NULL CHECK(kind IN('order','withdrawal','recovery','dispute')),
 reference text NOT NULL, amount_usd numeric(38,18) NOT NULL CHECK(amount_usd>0), restricted_entry_id uuid REFERENCES incentives_commission_entries(id),
 attempt integer NOT NULL CHECK(attempt>0), state text NOT NULL CHECK(state IN('reserved','consumed','released')), created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(operator_id,customer_id,kind,reference)
);
CREATE TABLE incentives_commission_funding_allocations (
 id uuid PRIMARY KEY, command_id uuid NOT NULL REFERENCES incentives_commission_funding_commands(id), entry_id uuid NOT NULL REFERENCES incentives_commission_funding_lots(entry_id),
 attempt integer NOT NULL, amount_usd numeric(38,18) NOT NULL CHECK(amount_usd>0), restored_usd numeric(38,18) NOT NULL DEFAULT 0 CHECK(restored_usd>=0 AND restored_usd<=amount_usd),
 state text NOT NULL CHECK(state IN('reserved','consumed','released')), created_at timestamptz NOT NULL DEFAULT clock_timestamp(), UNIQUE(command_id,attempt,entry_id)
);
CREATE TABLE incentives_commission_funding_restorations (
 id uuid PRIMARY KEY, operator_id uuid NOT NULL, customer_id uuid NOT NULL, order_id uuid NOT NULL, reference text NOT NULL,
 amount_usd numeric(38,18) NOT NULL CHECK(amount_usd>0), allocations jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp(), UNIQUE(order_id,reference)
);
CREATE TRIGGER incentives_commission_restoration_immutable BEFORE UPDATE OR DELETE ON incentives_commission_funding_restorations FOR EACH ROW EXECUTE FUNCTION incentives_immutable_fact();
CREATE TABLE incentives_commission_source_flags (
 event_id uuid NOT NULL REFERENCES incentives_confirmation_events(id), reference text NOT NULL, operator_id uuid NOT NULL,
 numerator_usd numeric(38,18) NOT NULL CHECK(numerator_usd>0), denominator_usd numeric(38,18) NOT NULL CHECK(denominator_usd>0 AND numerator_usd<=denominator_usd),
 active boolean NOT NULL, evidence text NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp(), updated_at timestamptz NOT NULL DEFAULT clock_timestamp(), PRIMARY KEY(event_id,reference)
);
CREATE TABLE incentives_commission_order_flags (
 order_id uuid NOT NULL, reference text NOT NULL, operator_id uuid NOT NULL,
 numerator_usd numeric(38,18) NOT NULL CHECK(numerator_usd>0), denominator_usd numeric(38,18) NOT NULL CHECK(denominator_usd>0 AND numerator_usd<=denominator_usd),
 active boolean NOT NULL, future_events boolean NOT NULL, evidence text NOT NULL, fingerprint text NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp(), PRIMARY KEY(order_id,reference)
);
CREATE TABLE incentives_commission_entry_flags (
 entry_id uuid NOT NULL REFERENCES incentives_commission_entries(id), reference text NOT NULL, event_id uuid NOT NULL,
 blocked_usd numeric(38,18) NOT NULL CHECK(blocked_usd>=0), active boolean NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp(), PRIMARY KEY(entry_id,reference)
);
CREATE TABLE incentives_commission_dispute_holds (
 entry_id uuid NOT NULL REFERENCES incentives_commission_entries(id), reference text NOT NULL, operator_id uuid NOT NULL,
 hold_id uuid NOT NULL REFERENCES ledger_holds(id), amount_usd numeric(38,18) NOT NULL CHECK(amount_usd>0), state text NOT NULL CHECK(state IN('held','released')),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), PRIMARY KEY(entry_id,reference)
);
CREATE FUNCTION incentives_preserve_commission_lot() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' OR (to_jsonb(NEW)-'operator_id'-'reserved_usd'-'consumed_usd') IS DISTINCT FROM (to_jsonb(OLD)-'operator_id'-'reserved_usd'-'consumed_usd') THEN RAISE EXCEPTION 'commission funding origin is immutable' USING ERRCODE='23514'; END IF; RETURN NEW;
END $$;
CREATE TRIGGER incentives_commission_lot_history BEFORE UPDATE OR DELETE ON incentives_commission_funding_lots FOR EACH ROW EXECUTE FUNCTION incentives_preserve_commission_lot();
