-- 管理员 TOTP。密钥只存密文；未启用前敏感操作仍只要求二次确认。
CREATE TABLE IF NOT EXISTS identity_admin_totp (
    user_id TEXT PRIMARY KEY REFERENCES identity_users(id),
    secret_ciphertext TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    enabled_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
