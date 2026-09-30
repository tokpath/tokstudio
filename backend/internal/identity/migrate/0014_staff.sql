INSERT INTO identity_roles (id, code) VALUES
    ('role_oem_ops', 'oem_ops'),
    ('role_oem_finance', 'oem_finance'),
    ('role_oem_audit', 'oem_audit')
ON CONFLICT DO NOTHING;

-- Employees belong to one company. Customer attribution remains separate.
CREATE TABLE identity_staff_members (
    user_id TEXT PRIMARY KEY REFERENCES identity_users(id),
    scope_type TEXT NOT NULL CHECK (scope_type IN ('platform', 'channel')),
    scope_id TEXT NOT NULL,
    roles_json JSONB NOT NULL,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    created_by TEXT NOT NULL REFERENCES identity_users(id),
    updated_by TEXT NOT NULL REFERENCES identity_users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX identity_staff_scope ON identity_staff_members (scope_type, scope_id);

INSERT INTO identity_staff_members (user_id, scope_type, scope_id, roles_json, created_by, updated_by)
SELECT ur.user_id, ur.scope_type, ur.scope_id, jsonb_agg(r.code ORDER BY r.code), ur.user_id, ur.user_id
FROM identity_user_roles ur
JOIN identity_roles r ON r.id = ur.role_id
WHERE (ur.scope_type = 'platform' AND ur.scope_id = '*' AND r.code IN ('platform_admin', 'finance_admin', 'ops_admin', 'tech_admin', 'audit_readonly'))
   OR (ur.scope_type = 'channel' AND r.code = 'channel_admin' AND ur.scope_id IN (SELECT id FROM identity_channel_orgs WHERE type = 'C'))
GROUP BY ur.user_id, ur.scope_type, ur.scope_id
ON CONFLICT DO NOTHING;
