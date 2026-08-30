-- 推广主体与登录用户的绑定。代理商/KOL 仍是普通终端用户，另挂 acquisition_role。
CREATE TABLE IF NOT EXISTS identity_role_members (
    user_id TEXT NOT NULL REFERENCES identity_users(id),
    acquisition_role_id TEXT NOT NULL REFERENCES identity_acquisition_roles(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, acquisition_role_id)
);

CREATE INDEX IF NOT EXISTS identity_role_members_role_idx
    ON identity_role_members (acquisition_role_id);
