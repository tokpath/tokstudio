-- Retain trusted signup brand and a validated site-local destination in OAuth state.
ALTER TABLE identity_oauth_states ADD COLUMN IF NOT EXISTS brand_id TEXT;
ALTER TABLE identity_oauth_states ADD COLUMN IF NOT EXISTS return_path TEXT;
ALTER TABLE identity_oauth_states ADD COLUMN IF NOT EXISTS consumed_at TIMESTAMPTZ;
