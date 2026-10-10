CREATE TABLE identity_api_key_creations (
    user_id text NOT NULL REFERENCES identity_users(id),
    operation_id text NOT NULL,
    api_key_id text NOT NULL REFERENCES identity_api_keys(id),
    fingerprint text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, operation_id)
);
