CREATE TABLE billing_notification_marks (
    key TEXT PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX billing_preauth_request_idx ON billing_preauth_failures(user_id, request_id, reason);
CREATE TABLE billing_quota_notification_state (
    owner_id TEXT PRIMARY KEY,
    low BOOLEAN NOT NULL DEFAULT false
);
