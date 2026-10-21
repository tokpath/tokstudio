ALTER TABLE tokenhub_provider_credentials
  ADD COLUMN IF NOT EXISTS name text NOT NULL DEFAULT '';
