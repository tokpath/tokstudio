CREATE TABLE commerce_manual_gifts(
 id uuid PRIMARY KEY,operator_id uuid NOT NULL REFERENCES organization_operators(id),original_operator_id uuid NOT NULL REFERENCES organization_operators(id),
 customer_id uuid NOT NULL REFERENCES organization_customers(id),grant_id uuid NOT NULL UNIQUE REFERENCES ledger_grants(id),
 ownership_version bigint NOT NULL,original_ownership_version bigint NOT NULL,
 business_key text NOT NULL,fingerprint text NOT NULL,amount_atg numeric(38,18) NOT NULL CHECK(amount_atg>0),
 reason text NOT NULL CHECK(reason IN('compensation','targeted_gift')),evidence text NOT NULL CHECK(evidence<>''),actor_id uuid NOT NULL,
 coverage_reference text NOT NULL,coverage_version bigint NOT NULL,effective_at timestamptz NOT NULL,expires_at timestamptz,created_at timestamptz NOT NULL,
 UNIQUE(operator_id,customer_id,business_key),CHECK(expires_at IS NULL OR expires_at>effective_at)
);
CREATE INDEX commerce_manual_gift_history ON commerce_manual_gifts(operator_id,customer_id,created_at DESC,id DESC);
CREATE FUNCTION commerce_preserve_manual_gift() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' OR (to_jsonb(NEW)-'operator_id'-'ownership_version') IS DISTINCT FROM (to_jsonb(OLD)-'operator_id'-'ownership_version') THEN RAISE EXCEPTION 'manual gift source is immutable'; END IF; RETURN NEW;
END $$;
CREATE TRIGGER commerce_manual_gift_immutable BEFORE UPDATE OR DELETE ON commerce_manual_gifts FOR EACH ROW EXECUTE FUNCTION commerce_preserve_manual_gift();
