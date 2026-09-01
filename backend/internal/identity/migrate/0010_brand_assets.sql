-- OEM 品牌资源：公开长期对象，不走媒体 7 天签名 URL。
ALTER TABLE identity_brands ADD COLUMN IF NOT EXISTS favicon_url TEXT;
ALTER TABLE identity_brands ADD COLUMN IF NOT EXISTS logo_dark_url TEXT;

CREATE TABLE IF NOT EXISTS identity_brand_assets (
    id TEXT PRIMARY KEY,
    brand_id TEXT NOT NULL REFERENCES identity_brands(id),
    kind TEXT NOT NULL,
    object_key TEXT NOT NULL,
    content_type TEXT NOT NULL,
    size_bytes BIGINT NOT NULL,
    width_px INTEGER NOT NULL DEFAULT 0,
    height_px INTEGER NOT NULL DEFAULT 0,
    sha256 TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    created_by TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS identity_brand_assets_brand_kind_idx
    ON identity_brand_assets (brand_id, kind, status);
