-- 身份模块自己的表。其他模块不得直接 JOIN 或复用这些 Model。
CREATE TABLE IF NOT EXISTS identity_users (
    id TEXT PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT,
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS identity_roles (
    id TEXT PRIMARY KEY,
    code TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS identity_user_roles (
    user_id TEXT NOT NULL REFERENCES identity_users(id),
    role_id TEXT NOT NULL REFERENCES identity_roles(id),
    scope_type TEXT NOT NULL DEFAULT 'platform',
    scope_id TEXT NOT NULL DEFAULT '*',
    PRIMARY KEY (user_id, role_id, scope_type, scope_id)
);

CREATE TABLE IF NOT EXISTS identity_access_tokens (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES identity_users(id),
    token_hash TEXT NOT NULL UNIQUE,
    prefix TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO identity_roles (id, code) VALUES
    ('role_platform_admin', 'platform_admin'),
    ('role_finance_admin', 'finance_admin'),
    ('role_ops_admin', 'ops_admin'),
    ('role_tech_admin', 'tech_admin'),
    ('role_channel_admin', 'channel_admin'),
    ('role_audit_readonly', 'audit_readonly'),
    ('role_end_user', 'end_user')
ON CONFLICT (id) DO NOTHING;
