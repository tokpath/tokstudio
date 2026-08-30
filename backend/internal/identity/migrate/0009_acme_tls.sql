-- OEM 证书签发记录 ACME 目录与过期时间。空目录时仍走沙箱 issued，不假装公网 Let's Encrypt。
ALTER TABLE identity_brands
    ADD COLUMN IF NOT EXISTS tls_issuer TEXT,
    ADD COLUMN IF NOT EXISTS tls_directory TEXT,
    ADD COLUMN IF NOT EXISTS tls_expires_at TIMESTAMPTZ;
