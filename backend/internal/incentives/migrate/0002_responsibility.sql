-- Economic fields and acquisition history remain immutable; only the current
-- fulfilment scope can change in a fully audited channel takeover transaction.
DROP TRIGGER incentives_function_immutable ON incentives_function_states;
DROP TRIGGER incentives_confirmation_history ON incentives_confirmation_events;
DO $$ DECLARE t TEXT; BEGIN
 FOREACH t IN ARRAY ARRAY['incentives_qualification_accounts','incentives_function_states','incentives_campaigns','incentives_order_snapshots','incentives_confirmation_events','incentives_commission_entries','incentives_commission_carries','incentives_corrections','incentives_offsets','incentives_withdrawals','incentives_registration_promises','incentives_kol_customer_counts','incentives_combinations'] LOOP
 EXECUTE format('ALTER TABLE %I ADD COLUMN original_operator_id UUID',t);
 EXECUTE format('UPDATE %I SET original_operator_id=operator_id',t);
 EXECUTE format('ALTER TABLE %I ALTER COLUMN original_operator_id SET NOT NULL',t);
 END LOOP; END $$;
CREATE FUNCTION incentives_preserve_origin() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='INSERT' THEN NEW.original_operator_id:=COALESCE(NEW.original_operator_id,NEW.operator_id);
 ELSIF NEW.original_operator_id<>OLD.original_operator_id THEN RAISE EXCEPTION 'original incentive responsibility is immutable' USING ERRCODE='23514'; END IF; RETURN NEW; END $$;
DO $$ DECLARE t TEXT; BEGIN
 FOREACH t IN ARRAY ARRAY['incentives_qualification_accounts','incentives_function_states','incentives_campaigns','incentives_order_snapshots','incentives_confirmation_events','incentives_commission_entries','incentives_commission_carries','incentives_corrections','incentives_offsets','incentives_withdrawals','incentives_registration_promises','incentives_kol_customer_counts','incentives_combinations'] LOOP
 EXECUTE format('CREATE TRIGGER incentives_origin BEFORE INSERT OR UPDATE ON %I FOR EACH ROW EXECUTE FUNCTION incentives_preserve_origin()',t);
 END LOOP; END $$;
CREATE OR REPLACE FUNCTION incentives_preserve_confirmation() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'confirmation facts cannot be deleted' USING ERRCODE='23514'; END IF;
 IF (to_jsonb(NEW)-'status'-'operator_id') IS DISTINCT FROM (to_jsonb(OLD)-'status'-'operator_id') THEN RAISE EXCEPTION 'confirmation economics are immutable' USING ERRCODE='23514'; END IF; RETURN NEW; END $$;
CREATE TRIGGER incentives_confirmation_history BEFORE UPDATE OR DELETE ON incentives_confirmation_events FOR EACH ROW EXECUTE FUNCTION incentives_preserve_confirmation();
CREATE FUNCTION incentives_preserve_function() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' OR (to_jsonb(NEW)-'operator_id') IS DISTINCT FROM (to_jsonb(OLD)-'operator_id') THEN RAISE EXCEPTION 'function history is immutable' USING ERRCODE='23514'; END IF; RETURN NEW; END $$;
CREATE TRIGGER incentives_function_immutable BEFORE UPDATE OR DELETE ON incentives_function_states FOR EACH ROW EXECUTE FUNCTION incentives_preserve_function();
CREATE TABLE incentives_responsibility_transfers (
 takeover_id UUID PRIMARY KEY,from_operator_id UUID NOT NULL,to_operator_id UUID NOT NULL,
 fingerprint TEXT NOT NULL,actor_id TEXT NOT NULL,evidence TEXT NOT NULL,created_at TIMESTAMPTZ NOT NULL
);
ALTER TABLE incentives_withdrawals DROP CONSTRAINT incentives_withdrawals_operator_id_business_key_key;
ALTER TABLE incentives_withdrawals ADD UNIQUE(operator_id,customer_id,business_key);
ALTER TABLE incentives_withdrawals DROP CONSTRAINT incentives_withdrawals_operator_id_payment_reference_key;
ALTER TABLE incentives_withdrawals ADD UNIQUE(original_operator_id,payment_reference);
ALTER TABLE incentives_corrections DROP CONSTRAINT incentives_corrections_operator_id_business_key_key;
ALTER TABLE incentives_corrections ADD UNIQUE(operator_id,original_event_id,business_key);
CREATE TABLE incentives_responsibility_transfer_customers (
 takeover_id UUID NOT NULL REFERENCES incentives_responsibility_transfers(takeover_id),customer_id UUID NOT NULL,
 old_ownership_version BIGINT NOT NULL,PRIMARY KEY(takeover_id,customer_id)
);
