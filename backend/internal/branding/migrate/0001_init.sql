-- Depends on organization/0001_init.
CREATE TABLE branding_domains (
    id UUID PRIMARY KEY,
    brand_id UUID NOT NULL REFERENCES organization_brands(id),
    hostname TEXT NOT NULL UNIQUE CHECK(hostname=lower(hostname)),
    verification_token TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','verified','active','disabled','failed')),
    tls_ready BOOLEAN NOT NULL DEFAULT false,
    verified_at TIMESTAMPTZ,
    checked_at TIMESTAMPTZ,
    provider_reference TEXT NOT NULL DEFAULT '',
    failure_reason TEXT NOT NULL DEFAULT '',
    version BIGINT NOT NULL DEFAULT 1 CHECK(version>0),
    created_at TIMESTAMPTZ NOT NULL,
    CHECK(status<>'active' OR (verified_at IS NOT NULL AND tls_ready AND length(provider_reference)>0))
);
CREATE INDEX branding_active_domains ON branding_domains(hostname,brand_id) WHERE status='active';
CREATE TABLE branding_theme_configs (
    id UUID PRIMARY KEY,
    brand_id UUID NOT NULL REFERENCES organization_brands(id),
    scope_type TEXT NOT NULL CHECK(scope_type IN ('brand','channel')),
    scope_id UUID NOT NULL,
    values_json JSONB NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(values_json)='object'),
    version BIGINT NOT NULL CHECK(version>0),
    updated_by UUID NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE(scope_type,scope_id)
);
CREATE FUNCTION branding_validate_theme_scope() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.scope_type='brand' THEN
        IF NEW.scope_id<>NEW.brand_id THEN RAISE EXCEPTION 'brand theme scope mismatch' USING ERRCODE='23514'; END IF;
    ELSE
        IF NOT EXISTS(SELECT 1 FROM organization_operators WHERE id=NEW.scope_id AND brand_id=NEW.brand_id AND kind='channel') THEN RAISE EXCEPTION 'channel theme scope mismatch' USING ERRCODE='23514'; END IF;
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER branding_theme_boundary BEFORE INSERT OR UPDATE ON branding_theme_configs FOR EACH ROW EXECUTE FUNCTION branding_validate_theme_scope();
CREATE TABLE branding_changes (
    id UUID PRIMARY KEY,
    brand_id UUID NOT NULL REFERENCES organization_brands(id),
    actor_id UUID NOT NULL,
    target_type TEXT NOT NULL,
    target_id UUID NOT NULL,
    action TEXT NOT NULL,
    before_json JSONB NOT NULL,
    after_json JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);
