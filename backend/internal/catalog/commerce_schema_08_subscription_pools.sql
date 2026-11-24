ALTER TABLE tokenhub_provider_credentials ADD COLUMN subscription_kind text NOT NULL DEFAULT ''
 CHECK (subscription_kind IN ('', 'codex', 'claude'));

CREATE TABLE tokenhub_subscription_accounts (
 id text PRIMARY KEY,
 pool_id text NOT NULL REFERENCES tokenhub_provider_credentials(id),
 name text NOT NULL,
 ciphertext text NOT NULL,
 identity_hash text NOT NULL,
 enabled boolean NOT NULL,
 removed boolean NOT NULL DEFAULT false,
 version bigint NOT NULL CHECK (version > 0),
 expires_at timestamptz,
 subscription_ends_at timestamptz,
 refreshable boolean NOT NULL,
 reauth_required boolean NOT NULL DEFAULT false,
 cooldown_until timestamptz,
 last_selected_at timestamptz,
 UNIQUE (pool_id, identity_hash)
);
CREATE INDEX tokenhub_subscription_accounts_pool ON tokenhub_subscription_accounts(pool_id, last_selected_at, id) WHERE enabled;

ALTER TABLE tokenhub_route_leases
 ADD COLUMN subscription_account_id text REFERENCES tokenhub_subscription_accounts(id) ON DELETE SET NULL,
 ADD COLUMN subscription_account_version bigint;
CREATE INDEX tokenhub_subscription_active_leases ON tokenhub_route_leases(subscription_account_id, expires_at)
 WHERE subscription_account_id IS NOT NULL AND finished_at IS NULL;
