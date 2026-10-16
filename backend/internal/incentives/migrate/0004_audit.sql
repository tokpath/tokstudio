CREATE FUNCTION incentives_preserve_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' OR (to_jsonb(NEW)-'published_at') IS DISTINCT FROM (to_jsonb(OLD)-'published_at') THEN RAISE EXCEPTION 'incentives audit is immutable' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER incentives_audit_immutable BEFORE UPDATE OR DELETE ON incentives_events FOR EACH ROW EXECUTE FUNCTION incentives_preserve_audit();
