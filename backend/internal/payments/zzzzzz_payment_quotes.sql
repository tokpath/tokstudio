CREATE TABLE IF NOT EXISTS payments_fx_settings (
 id BOOLEAN PRIMARY KEY DEFAULT true CHECK(id),
 source TEXT NOT NULL CHECK(source='frankfurter'),
 enabled BOOLEAN NOT NULL,
 version BIGINT NOT NULL,
 last_attempt_at TIMESTAMPTZ,
 error_code TEXT NOT NULL DEFAULT '',
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO payments_fx_settings(id,source,enabled,version) VALUES(true,'frankfurter',true,1) ON CONFLICT DO NOTHING;
CREATE TABLE IF NOT EXISTS payments_fx_rates (
 base_currency TEXT NOT NULL, quote_currency TEXT NOT NULL,
 rate NUMERIC(38,18) NOT NULL CHECK(rate>0),
 rate_date DATE NOT NULL, fetched_at TIMESTAMPTZ NOT NULL,
 attempted_at TIMESTAMPTZ NOT NULL, error_code TEXT NOT NULL DEFAULT '',
 PRIMARY KEY(base_currency,quote_currency)
);
