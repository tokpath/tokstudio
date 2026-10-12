CREATE TABLE mail_settings (
    id TEXT PRIMARY KEY,
    enabled BOOLEAN NOT NULL,
    payload_ciphertext TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
