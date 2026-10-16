CREATE FUNCTION commerce_preserve_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' OR (to_jsonb(NEW)-'published_at') IS DISTINCT FROM (to_jsonb(OLD)-'published_at') THEN RAISE EXCEPTION 'commerce audit is immutable' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER commerce_audit_immutable BEFORE UPDATE OR DELETE ON commerce_events FOR EACH ROW EXECUTE FUNCTION commerce_preserve_audit();
