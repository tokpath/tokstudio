-- Google sign-in is configured per brand. A subject identifies one customer
-- inside that brand and is never shared with another brand.
ALTER TABLE organization_customer_identities ADD COLUMN google_subject TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX organization_brand_google_subject ON organization_customer_identities(brand_id, google_subject) WHERE google_subject <> '';

CREATE TABLE organization_google_configs (
    brand_id UUID PRIMARY KEY REFERENCES organization_brands(id),
    client_id TEXT NOT NULL,
    secret_ciphertext TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT false,
    version BIGINT NOT NULL CHECK(version>0),
    verified_redirects JSONB NOT NULL DEFAULT '[]'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL
);
CREATE UNIQUE INDEX organization_google_client_id ON organization_google_configs(client_id);

CREATE TABLE organization_google_states (
    id UUID PRIMARY KEY,
    brand_id UUID NOT NULL REFERENCES organization_brands(id),
    state_hash TEXT NOT NULL UNIQUE,
    redirect_uri TEXT NOT NULL,
    invite_code TEXT NOT NULL DEFAULT '',
    next_path TEXT NOT NULL DEFAULT '',
    config_version BIGINT NOT NULL,
    test_only BOOLEAN NOT NULL DEFAULT false,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE organization_google_tests (
    id UUID PRIMARY KEY,
    ticket_hash TEXT NOT NULL UNIQUE,
    brand_id UUID NOT NULL REFERENCES organization_brands(id),
    redirect_uri TEXT NOT NULL,
    config_version BIGINT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL
);
