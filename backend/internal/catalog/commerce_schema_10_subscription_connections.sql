ALTER TABLE tokenhub_subscription_accounts
    ADD COLUMN connection_hash text NOT NULL DEFAULT '',
    ADD COLUMN discovered_models jsonb NOT NULL DEFAULT '[]'::jsonb;
