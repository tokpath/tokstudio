LOCK TABLE usage_requests IN SHARE ROW EXCLUSIVE MODE;

CREATE TABLE usage_key_budget_totals (
    key_id uuid NOT NULL REFERENCES usage_keys(id),
    bucket_start timestamptz NOT NULL,
    spent_atg numeric NOT NULL CHECK (spent_atg >= 0),
    held_atg numeric NOT NULL CHECK (held_atg >= 0),
    PRIMARY KEY (key_id, bucket_start)
);

CREATE FUNCTION usage_budget_held(request_status text, estimate numeric) RETURNS numeric
LANGUAGE sql IMMUTABLE STRICT AS $$
    SELECT CASE WHEN request_status IN ('accepted','executing','submitted','awaiting_usage') THEN estimate ELSE 0 END
$$;

-- One lifetime total and one UTC hour per request. Exact calendar windows
-- combine complete hours with request rows from their two boundary hours.
WITH hourly AS (
    SELECT key_id, date_bin(interval '1 hour', accepted_at, timestamptz '2000-01-01 UTC') AS bucket_start,
        sum(actual_atg - corrected_atg) AS spent_atg,
        sum(usage_budget_held(status, estimate_atg)) AS held_atg
    FROM usage_requests
    GROUP BY key_id, 2
)
INSERT INTO usage_key_budget_totals (key_id, bucket_start, spent_atg, held_atg)
SELECT key_id, bucket_start, spent_atg, held_atg FROM hourly
UNION ALL
SELECT key_id, '-infinity'::timestamptz, sum(spent_atg), sum(held_atg) FROM hourly GROUP BY key_id;

CREATE FUNCTION usage_update_key_budget() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    spent numeric := NEW.actual_atg - NEW.corrected_atg;
    held numeric := usage_budget_held(NEW.status, NEW.estimate_atg);
    bucket timestamptz := date_bin(interval '1 hour', NEW.accepted_at, timestamptz '2000-01-01 UTC');
BEGIN
    IF TG_OP = 'UPDATE' THEN
        spent := spent - (OLD.actual_atg - OLD.corrected_atg);
        held := held - usage_budget_held(OLD.status, OLD.estimate_atg);
    END IF;
    IF spent = 0 AND held = 0 THEN
        RETURN NULL;
    END IF;

    INSERT INTO usage_key_budget_totals (key_id, bucket_start, spent_atg, held_atg)
    VALUES (NEW.key_id, '-infinity', 0, 0), (NEW.key_id, bucket, 0, 0)
    ON CONFLICT DO NOTHING;

    UPDATE usage_key_budget_totals
    SET spent_atg = spent_atg + spent, held_atg = held_atg + held
    WHERE key_id = NEW.key_id AND bucket_start IN ('-infinity'::timestamptz, bucket);
    RETURN NULL;
END;
$$;

CREATE TRIGGER usage_key_budget_totals_changed
AFTER INSERT OR UPDATE OF status, estimate_atg, actual_atg, corrected_atg ON usage_requests
FOR EACH ROW EXECUTE FUNCTION usage_update_key_budget();
