CREATE TABLE payments_asset_network_versions(
 version_id UUID PRIMARY KEY,id UUID NOT NULL,version BIGINT NOT NULL CHECK(version>0),
 driver TEXT NOT NULL CHECK(driver IN('evm_wallet','nowpayments')),asset TEXT NOT NULL,network TEXT NOT NULL,token_contract TEXT NOT NULL,
 token_decimals INTEGER NOT NULL CHECK(token_decimals>=0 AND token_decimals<=18),minimum_confirmations BIGINT NOT NULL CHECK(minimum_confirmations>=0),
 enabled BOOLEAN NOT NULL,effective_at TIMESTAMPTZ NOT NULL,evidence_reference TEXT NOT NULL,published_by TEXT NOT NULL,published_at TIMESTAMPTZ NOT NULL,
 UNIQUE(id,version)
);
CREATE INDEX payments_asset_network_active ON payments_asset_network_versions(id,effective_at DESC,version DESC);
CREATE FUNCTION payments_immutable_asset_version() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'supported asset/network versions are immutable';END$$;
CREATE TRIGGER payments_asset_network_versions_immutable BEFORE UPDATE OR DELETE ON payments_asset_network_versions FOR EACH ROW EXECUTE FUNCTION payments_immutable_asset_version();
