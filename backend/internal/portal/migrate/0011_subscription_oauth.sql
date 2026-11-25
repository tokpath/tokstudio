CREATE TABLE portal_subscription_oauth (
 state_hash text PRIMARY KEY,
 session_hash text NOT NULL REFERENCES portal_sessions(token_hash) ON DELETE CASCADE,
 supplier_kind text NOT NULL,
 redirect_uri text NOT NULL,
 connection_hash text NOT NULL,
 ciphertext text NOT NULL,
 expires_at timestamptz NOT NULL
);
CREATE INDEX portal_subscription_oauth_expiry ON portal_subscription_oauth(expires_at);
CREATE INDEX portal_subscription_oauth_session ON portal_subscription_oauth(session_hash);
