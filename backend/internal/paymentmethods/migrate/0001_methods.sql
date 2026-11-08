CREATE TABLE payment_method_versions (
 id UUID PRIMARY KEY,
 method_id TEXT NOT NULL,
 operator_id TEXT NOT NULL,
 version BIGINT NOT NULL CHECK(version>0),
 name TEXT NOT NULL CHECK(length(trim(name))>0),
 collection BOOLEAN NOT NULL DEFAULT false,
 payout_enabled BOOLEAN NOT NULL DEFAULT false,
 payout_kind TEXT NOT NULL DEFAULT '',
 flat_fee_money NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK(flat_fee_money>=0),
 fee_rate NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK(fee_rate>=0 AND fee_rate<1),
 created_by TEXT NOT NULL DEFAULT 'migration',
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 CHECK(flat_fee_money=0 OR fee_rate=0),
 CHECK(NOT payout_enabled OR payout_kind<>''),
 UNIQUE(operator_id,method_id,version)
);
CREATE INDEX payment_method_current ON payment_method_versions(operator_id,method_id,version DESC);
CREATE FUNCTION payment_method_immutable() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'payment method versions are immutable' USING ERRCODE='23514'; END $$;
CREATE TRIGGER payment_method_version_immutable BEFORE UPDATE OR DELETE ON payment_method_versions FOR EACH ROW EXECUTE FUNCTION payment_method_immutable();
