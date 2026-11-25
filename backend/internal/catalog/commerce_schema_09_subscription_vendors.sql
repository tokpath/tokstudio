ALTER TABLE tokenhub_provider_credentials
 DROP CONSTRAINT tokenhub_provider_credentials_subscription_kind_check,
 ADD CONSTRAINT tokenhub_provider_credentials_subscription_kind_check
 CHECK (subscription_kind IN ('', 'codex', 'claude', 'grok', 'gemini', 'antigravity'));
