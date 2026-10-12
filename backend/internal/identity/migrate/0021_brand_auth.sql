-- Preserve existing user IDs and their financial history. Unattributed legacy
-- accounts must be resolved before opening registration to duplicate emails.
UPDATE identity_users u SET brand_id = c.brand_id
FROM identity_channel_orgs c
WHERE u.channel_org_id = c.id AND u.brand_id IS NULL;
UPDATE identity_users u SET brand_id = 'brd_official'
WHERE u.brand_id IS NULL AND EXISTS (
    SELECT 1 FROM identity_user_roles ur WHERE ur.user_id = u.id AND ur.scope_type = 'platform'
);
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM identity_users WHERE brand_id IS NULL) THEN
        RAISE EXCEPTION 'Assign a brand to every legacy account before applying brand authentication';
    END IF;
END $$;
ALTER TABLE identity_users ALTER COLUMN brand_id SET NOT NULL;
ALTER TABLE identity_users DROP CONSTRAINT IF EXISTS identity_users_email_key;
ALTER TABLE identity_users DROP CONSTRAINT IF EXISTS identity_users_google_sub_key;
UPDATE identity_users SET email = lower(btrim(email));
CREATE UNIQUE INDEX identity_users_brand_email ON identity_users (brand_id, lower(email));
CREATE UNIQUE INDEX identity_users_brand_google ON identity_users (brand_id, google_sub) WHERE google_sub IS NOT NULL;

-- Legacy OTPs cannot be reused without a brand context.
ALTER TABLE identity_email_otps ADD COLUMN brand_id TEXT REFERENCES identity_brands(id);
UPDATE identity_email_otps SET consumed_at = now() WHERE consumed_at IS NULL;

CREATE TABLE identity_google_configs (
    brand_id TEXT PRIMARY KEY REFERENCES identity_brands(id),
    client_id TEXT NOT NULL UNIQUE,
    secret_ciphertext TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT false,
    version BIGINT NOT NULL DEFAULT 1,
    verification_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE identity_oauth_states ADD COLUMN config_version BIGINT NOT NULL DEFAULT 0;
ALTER TABLE identity_oauth_states ADD COLUMN redirect_uri TEXT NOT NULL DEFAULT '';
ALTER TABLE identity_oauth_states ADD COLUMN link_user_id TEXT REFERENCES identity_users(id);
ALTER TABLE identity_oauth_states ADD COLUMN test_only BOOLEAN NOT NULL DEFAULT false;
CREATE TABLE identity_google_test_tickets (
    ticket_hash TEXT PRIMARY KEY,
    brand_id TEXT NOT NULL REFERENCES identity_brands(id),
    config_version BIGINT NOT NULL,
    redirect_uri TEXT NOT NULL,
    created_by TEXT NOT NULL REFERENCES identity_users(id),
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ
);
