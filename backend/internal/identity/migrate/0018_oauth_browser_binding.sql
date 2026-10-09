-- A short-lived browser challenge binds every callback and replay to its initiator.
ALTER TABLE identity_oauth_states ADD COLUMN IF NOT EXISTS consumed_at TIMESTAMPTZ;
ALTER TABLE identity_oauth_states ADD COLUMN IF NOT EXISTS browser_hash TEXT;
