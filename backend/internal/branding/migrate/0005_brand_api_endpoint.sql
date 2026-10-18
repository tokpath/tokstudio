-- Select the first-added usable domain consistently for the brand API entry.
CREATE INDEX branding_brand_api_endpoint ON branding_domains(brand_id,created_at,id)
 WHERE status='active' AND verified_at IS NOT NULL AND tls_ready AND provider_reference<>'';
