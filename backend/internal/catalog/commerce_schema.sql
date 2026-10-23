CREATE TABLE IF NOT EXISTS tokenhub_catalog_revisions (
 id text PRIMARY KEY, version bigint NOT NULL CHECK(version>0)
);
INSERT INTO tokenhub_catalog_revisions(id,version) VALUES('global',1) ON CONFLICT DO NOTHING;
CREATE TABLE IF NOT EXISTS tokenhub_public_models (
 id text PRIMARY KEY, definition jsonb NOT NULL, version bigint NOT NULL CHECK(version>0)
);
CREATE TABLE IF NOT EXISTS tokenhub_upstreams (
 id text PRIMARY KEY, public_model_id text NOT NULL REFERENCES tokenhub_public_models(id),
 provider_id text NOT NULL, definition jsonb NOT NULL, version bigint NOT NULL CHECK(version>0)
);
CREATE INDEX IF NOT EXISTS tokenhub_upstreams_model ON tokenhub_upstreams(public_model_id);
CREATE TABLE IF NOT EXISTS tokenhub_provider_credentials (
 id text PRIMARY KEY, provider_id text NOT NULL, ciphertext text NOT NULL,
 fingerprint text NOT NULL, enabled boolean NOT NULL, version bigint NOT NULL CHECK(version>0)
);
CREATE TABLE IF NOT EXISTS tokenhub_model_authorizations (
 seller_id text NOT NULL, buyer_id text NOT NULL, public_model_id text NOT NULL REFERENCES tokenhub_public_models(id),
 enabled boolean NOT NULL, version bigint NOT NULL CHECK(version>0),
 PRIMARY KEY(seller_id,buyer_id,public_model_id), CHECK(seller_id<>buyer_id)
);
CREATE TABLE IF NOT EXISTS tokenhub_model_listings (
 operator_id text NOT NULL, public_model_id text NOT NULL REFERENCES tokenhub_public_models(id),
 enabled boolean NOT NULL, version bigint NOT NULL CHECK(version>0), PRIMARY KEY(operator_id,public_model_id)
);
CREATE TABLE IF NOT EXISTS tokenhub_model_price_versions (
 id text PRIMARY KEY, scope_key text NOT NULL, public_model_id text NOT NULL,
 layer text NOT NULL CHECK(layer IN ('regular','promotion')),
 effective_at timestamptz NOT NULL, ends_at timestamptz, definition jsonb NOT NULL,
 CHECK(ends_at IS NULL OR ends_at>effective_at), CHECK(layer<>'promotion' OR ends_at IS NOT NULL),
 UNIQUE(scope_key,public_model_id,layer,effective_at)
);
CREATE INDEX IF NOT EXISTS tokenhub_model_price_effective ON tokenhub_model_price_versions(scope_key,public_model_id,effective_at DESC);
CREATE TABLE IF NOT EXISTS tokenhub_catalog_changes(
 id text PRIMARY KEY,operator_id text NOT NULL,kind text NOT NULL,resource_id text NOT NULL,
 payload jsonb NOT NULL,fingerprint text NOT NULL,effective_at timestamptz NOT NULL,lead_seconds bigint NOT NULL,
 expected_version bigint NOT NULL,state text NOT NULL,evidence text NOT NULL,created_at timestamptz NOT NULL,
 announced_at timestamptz,applied_at timestamptz,error_code text NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS tokenhub_catalog_change_audiences(
 change_id text NOT NULL REFERENCES tokenhub_catalog_changes(id),operator_id text NOT NULL,cursor text NOT NULL DEFAULT '',done boolean NOT NULL DEFAULT false,
 PRIMARY KEY(change_id,operator_id)
);
CREATE TABLE IF NOT EXISTS tokenhub_catalog_change_notices(
 change_id text NOT NULL REFERENCES tokenhub_catalog_changes(id),operator_id text NOT NULL,recipient_id text NOT NULL,
 PRIMARY KEY(change_id,operator_id,recipient_id)
);
ALTER TABLE tokenhub_catalog_changes ADD COLUMN IF NOT EXISTS last_checked_at timestamptz NOT NULL DEFAULT '1970-01-01';
CREATE INDEX IF NOT EXISTS tokenhub_catalog_change_work ON tokenhub_catalog_changes(state,last_checked_at);
CREATE TABLE IF NOT EXISTS tokenhub_route_policies (
 model_id text PRIMARY KEY REFERENCES tokenhub_public_models(id),
 strategy text NOT NULL CHECK(strategy IN ('priority','lowest_latency','lowest_cost')),
 evidence text NOT NULL, version bigint NOT NULL CHECK(version>0)
);
CREATE TABLE IF NOT EXISTS tokenhub_route_health (
 upstream_id text PRIMARY KEY REFERENCES tokenhub_upstreams(id),
 circuit text NOT NULL CHECK(circuit IN ('closed','open')), epoch bigint NOT NULL DEFAULT 0,
 failures integer NOT NULL DEFAULT 0, unknown_usage integer NOT NULL DEFAULT 0,
 recovery_successes integer NOT NULL DEFAULT 0, latency_micros bigint NOT NULL DEFAULT 0,
 open_until timestamptz,updated_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE IF NOT EXISTS tokenhub_route_leases (
 attempt_id text PRIMARY KEY, upstream_id text NOT NULL REFERENCES tokenhub_upstreams(id),request_id text NOT NULL,
 epoch bigint NOT NULL,probe boolean NOT NULL,limits jsonb NOT NULL,
 started_at timestamptz NOT NULL,expires_at timestamptz NOT NULL,finished_at timestamptz
);
CREATE INDEX IF NOT EXISTS tokenhub_route_capacity ON tokenhub_route_leases(upstream_id,expires_at) WHERE finished_at IS NULL;
CREATE TABLE IF NOT EXISTS tokenhub_route_events (
 id uuid PRIMARY KEY,upstream_id text NOT NULL,kind text NOT NULL,reference text NOT NULL,data jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), UNIQUE(kind,reference)
);
CREATE OR REPLACE FUNCTION tokenhub_route_event_immutable() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 RAISE EXCEPTION 'route events are immutable'; END $$;
CREATE TRIGGER tokenhub_route_events_immutable BEFORE UPDATE OR DELETE ON tokenhub_route_events FOR EACH ROW EXECUTE FUNCTION tokenhub_route_event_immutable();
