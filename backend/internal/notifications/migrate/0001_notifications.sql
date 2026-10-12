CREATE TABLE notifications_inbox (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    brand_id TEXT NOT NULL,
    event_id TEXT NOT NULL,
    dedupe_key TEXT NOT NULL,
    kind TEXT NOT NULL,
    reference_id TEXT NOT NULL DEFAULT '',
    subject_user_id TEXT NOT NULL DEFAULT '',
    customer_channel_id TEXT NOT NULL DEFAULT '',
    portal TEXT NOT NULL DEFAULT 'app',
    period_end TIMESTAMPTZ,
    required_minor BIGINT NOT NULL DEFAULT 0,
    title TEXT NOT NULL,
    body TEXT NOT NULL,
    href TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    read_at TIMESTAMPTZ,
    UNIQUE (brand_id, user_id, dedupe_key)
);
CREATE INDEX notifications_inbox_user_idx ON notifications_inbox(user_id, brand_id, created_at DESC, id DESC);
CREATE INDEX notifications_inbox_unread_idx ON notifications_inbox(user_id, brand_id) WHERE read_at IS NULL;
CREATE TABLE notifications_email_deliveries (
    id TEXT PRIMARY KEY,
    notification_id TEXT NOT NULL UNIQUE REFERENCES notifications_inbox(id),
    status TEXT NOT NULL,
    payload_ciphertext TEXT NOT NULL DEFAULT '',
    attempts INTEGER NOT NULL DEFAULT 0,
    available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    lease_id TEXT,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    accepted_at TIMESTAMPTZ
);
CREATE INDEX notifications_email_queue_idx ON notifications_email_deliveries(status, available_at);
CREATE TABLE notifications_preferences (
    user_id TEXT PRIMARY KEY,
    brand_id TEXT NOT NULL,
    email_enabled BOOLEAN NOT NULL DEFAULT true,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
