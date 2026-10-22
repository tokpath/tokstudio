CREATE TABLE IF NOT EXISTS tokenhub_supplier_models (
 id text PRIMARY KEY, provider_id text NOT NULL, definition jsonb NOT NULL, version bigint NOT NULL CHECK(version>0)
);
CREATE INDEX IF NOT EXISTS tokenhub_supplier_models_provider ON tokenhub_supplier_models(provider_id);
