ALTER TABLE commerce_packages ADD COLUMN source_operator_id UUID REFERENCES organization_operators(id);
ALTER TABLE commerce_packages ADD COLUMN source_package_id UUID REFERENCES commerce_packages(id);
ALTER TABLE commerce_packages ADD COLUMN takeover_id UUID;
CREATE UNIQUE INDEX commerce_takeover_package_once ON commerce_packages(takeover_id,source_package_id) WHERE takeover_id IS NOT NULL;
