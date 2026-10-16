CREATE FUNCTION organization_preserve_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'organization audit is immutable' USING ERRCODE='23514'; END $$;
CREATE TRIGGER organization_audit_immutable BEFORE UPDATE OR DELETE ON organization_events FOR EACH ROW EXECUTE FUNCTION organization_preserve_audit();
