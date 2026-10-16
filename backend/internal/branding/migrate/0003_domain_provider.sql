ALTER TABLE branding_domains ADD COLUMN provider_status TEXT NOT NULL DEFAULT '';
ALTER TABLE branding_domains ADD COLUMN tls_status TEXT NOT NULL DEFAULT '';
ALTER TABLE branding_domains ADD COLUMN provider_instructions JSONB NOT NULL DEFAULT '[]' CHECK(jsonb_typeof(provider_instructions)='array');
