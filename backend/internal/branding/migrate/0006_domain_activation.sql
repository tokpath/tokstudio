ALTER TABLE branding_domains ADD COLUMN route_ready BOOLEAN NOT NULL DEFAULT false;
-- Previous automatic activation lacked platform approval and an application
-- routing check. Keep registrations, but require the new activation process.
UPDATE branding_domains SET status='verified', version=version+1 WHERE status='active';
ALTER TABLE branding_domains ADD CONSTRAINT branding_domain_route_ready CHECK(status<>'active' OR route_ready);
CREATE UNIQUE INDEX branding_one_active_domain ON branding_domains(brand_id) WHERE status='active';
