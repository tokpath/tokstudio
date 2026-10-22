CREATE TABLE model_pricing_settings (
 id TEXT PRIMARY KEY CHECK (id='platform'),
 enabled BOOLEAN NOT NULL DEFAULT TRUE,
 source_url TEXT NOT NULL DEFAULT 'https://getbifrost.ai/datasheet',
 interval_hours INTEGER NOT NULL DEFAULT 24 CHECK (interval_hours BETWEEN 1 AND 720),
 version BIGINT NOT NULL DEFAULT 1,
 sheet_version TEXT NOT NULL DEFAULT '',
 last_success_at TIMESTAMPTZ,
 last_attempt_at TIMESTAMPTZ,
 last_error TEXT NOT NULL DEFAULT '',
 sync_requested BOOLEAN NOT NULL DEFAULT TRUE
);
INSERT INTO model_pricing_settings(id) VALUES('platform');
CREATE TABLE model_pricing_rows (
 id TEXT PRIMARY KEY,
 provider TEXT NOT NULL,
 model TEXT NOT NULL,
 mode TEXT NOT NULL,
 definition JSONB NOT NULL
);
CREATE INDEX model_pricing_lookup ON model_pricing_rows(provider,model);
CREATE TABLE model_actual_costs (
 id TEXT PRIMARY KEY,
 provider_id TEXT NOT NULL,
 supplier_model_id TEXT NOT NULL DEFAULT '',
 starts_at TIMESTAMPTZ NOT NULL,
 ends_at TIMESTAMPTZ NOT NULL CHECK(ends_at>starts_at),
 amount_usd NUMERIC(38,18) NOT NULL CHECK(amount_usd>=0),
 note TEXT NOT NULL DEFAULT '',
 version BIGINT NOT NULL DEFAULT 1,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
