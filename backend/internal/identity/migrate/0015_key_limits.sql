-- Historical keys retain their policy, expiry, and consumption facts.
ALTER TABLE identity_api_keys ADD COLUMN model_mode TEXT NOT NULL DEFAULT 'all' CHECK (model_mode IN ('all','selected'));
UPDATE identity_api_keys SET model_mode='selected' WHERE EXISTS (SELECT 1 FROM identity_api_key_model_policies p WHERE p.api_key_id=identity_api_keys.id AND p.allowed=true);
ALTER TABLE identity_api_keys ADD COLUMN budget_limit_minor BIGINT CHECK (budget_limit_minor > 0);
ALTER TABLE identity_api_keys ADD COLUMN budget_used_minor BIGINT NOT NULL DEFAULT 0 CHECK (budget_used_minor >= 0);
ALTER TABLE identity_api_keys ADD COLUMN budget_reserved_minor BIGINT NOT NULL DEFAULT 0 CHECK (budget_reserved_minor >= 0);
