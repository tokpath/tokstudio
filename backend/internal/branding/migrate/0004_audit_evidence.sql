ALTER TABLE branding_changes ADD COLUMN evidence TEXT NOT NULL DEFAULT '';
CREATE FUNCTION branding_preserve_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'branding audit is immutable' USING ERRCODE='23514'; END $$;
CREATE TRIGGER branding_audit_immutable BEFORE UPDATE OR DELETE ON branding_changes FOR EACH ROW EXECUTE FUNCTION branding_preserve_audit();
