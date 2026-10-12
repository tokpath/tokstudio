-- Recovery secrets stay inside identity; they are never broadcast through Redis.
CREATE TABLE identity_email_actions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES identity_users(id),
    brand_id TEXT NOT NULL REFERENCES identity_brands(id),
    email TEXT NOT NULL,
    purpose TEXT NOT NULL CHECK (purpose IN ('verify_email', 'reset_password')),
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX identity_email_actions_user ON identity_email_actions(user_id, purpose, created_at DESC);
CREATE TABLE identity_email_deliveries (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES identity_users(id),
    action_id TEXT REFERENCES identity_email_actions(id),
    payload_ciphertext TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    attempts INTEGER NOT NULL DEFAULT 0,
    available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    lease_id TEXT,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    sent_at TIMESTAMPTZ
);
CREATE INDEX identity_email_deliveries_pending ON identity_email_deliveries(available_at) WHERE status IN ('pending', 'sending');
