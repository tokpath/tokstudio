-- API Key 属于身份模块。网关只通过 LookupAPIKey 接口拿到用户/渠道/白名单。
CREATE TABLE IF NOT EXISTS identity_api_keys (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES identity_users(id),
    name TEXT NOT NULL,
    prefix TEXT NOT NULL,
    secret_hash TEXT NOT NULL UNIQUE,
    secret_ciphertext TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    rpm_limit INTEGER NOT NULL DEFAULT 60,
    concurrency_limit INTEGER NOT NULL DEFAULT 5,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS identity_api_key_model_policies (
    api_key_id TEXT NOT NULL REFERENCES identity_api_keys(id),
    public_model_id TEXT NOT NULL,
    allowed BOOLEAN NOT NULL DEFAULT true,
    PRIMARY KEY (api_key_id, public_model_id)
);
