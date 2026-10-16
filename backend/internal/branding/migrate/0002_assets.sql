CREATE TABLE branding_logo_assets (
    id UUID PRIMARY KEY,
    brand_id UUID NOT NULL REFERENCES organization_brands(id),
    scope_type TEXT NOT NULL CHECK(scope_type IN ('brand','channel')),
    scope_id UUID NOT NULL,
    object_key TEXT NOT NULL UNIQUE,
    filename TEXT NOT NULL,
    content_type TEXT NOT NULL CHECK(content_type IN ('image/png','image/jpeg','image/webp')),
    size BIGINT NOT NULL CHECK(size>0 AND size<=2097152),
    sha256 TEXT NOT NULL CHECK(length(sha256)=64),
    width INTEGER NOT NULL CHECK(width>0),
    height INTEGER NOT NULL CHECK(height>0),
    status TEXT NOT NULL CHECK(status IN ('uploading','ready','failed')),
    uploaded_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CHECK(width::bigint*height::bigint<=10000000)
);
CREATE INDEX branding_logo_scope ON branding_logo_assets(scope_type,scope_id,created_at,id);
