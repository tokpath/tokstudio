-- M1：品牌、渠道、归因、验证码和 OAuth 状态。用户归属渠道后不可自行修改。
CREATE TABLE IF NOT EXISTS identity_brands (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    logo_url TEXT,
    primary_domain TEXT NOT NULL UNIQUE,
    api_domain TEXT NOT NULL,
    admin_domain TEXT NOT NULL,
    theme_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS identity_channel_orgs (
    id TEXT PRIMARY KEY,
    code TEXT NOT NULL UNIQUE,
    type TEXT NOT NULL,
    parent_id TEXT,
    status TEXT NOT NULL DEFAULT 'active',
    brand_id TEXT NOT NULL REFERENCES identity_brands(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS identity_acquisition_roles (
    id TEXT PRIMARY KEY,
    channel_org_id TEXT NOT NULL REFERENCES identity_channel_orgs(id),
    type TEXT NOT NULL,
    parent_id TEXT,
    level INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS identity_promotion_codes (
    id TEXT PRIMARY KEY,
    code TEXT NOT NULL UNIQUE,
    channel_org_id TEXT NOT NULL REFERENCES identity_channel_orgs(id),
    acquisition_role_id TEXT REFERENCES identity_acquisition_roles(id),
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS identity_attributions (
    user_id TEXT PRIMARY KEY REFERENCES identity_users(id),
    channel_org_id TEXT NOT NULL REFERENCES identity_channel_orgs(id),
    acquisition_role_id TEXT,
    source_code TEXT NOT NULL,
    attributed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS identity_attribution_changes (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES identity_users(id),
    before_json JSONB NOT NULL,
    after_json JSONB NOT NULL,
    reason TEXT NOT NULL,
    actor_user_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS identity_email_otps (
    id TEXT PRIMARY KEY,
    email TEXT NOT NULL,
    purpose TEXT NOT NULL,
    code_hash TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS identity_oauth_states (
    id TEXT PRIMARY KEY,
    state_hash TEXT NOT NULL UNIQUE,
    promotion_code TEXT,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE identity_users ADD COLUMN IF NOT EXISTS channel_org_id TEXT REFERENCES identity_channel_orgs(id);
ALTER TABLE identity_users ADD COLUMN IF NOT EXISTS brand_id TEXT REFERENCES identity_brands(id);
ALTER TABLE identity_users ADD COLUMN IF NOT EXISTS email_verified_at TIMESTAMPTZ;
ALTER TABLE identity_users ADD COLUMN IF NOT EXISTS google_sub TEXT UNIQUE;
