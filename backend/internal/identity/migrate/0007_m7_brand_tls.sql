-- OEM 域名 CNAME 接入后由边缘层自动签发 HTTPS。
ALTER TABLE identity_brands ADD COLUMN IF NOT EXISTS cname_target TEXT;
ALTER TABLE identity_brands ADD COLUMN IF NOT EXISTS tls_status TEXT NOT NULL DEFAULT 'pending';
UPDATE identity_brands SET cname_target = 'edge.tokenhub.local' WHERE cname_target IS NULL OR cname_target = '';
