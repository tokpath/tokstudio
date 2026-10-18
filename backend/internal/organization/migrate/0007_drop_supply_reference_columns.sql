-- These columns only stored synthetic clause numbers after readable terms
-- moved the real agreement onto supply_terms, exit_policy and the contract.
ALTER TABLE organization_supply_agreements DROP COLUMN quote_reference;
ALTER TABLE organization_supply_agreements DROP COLUMN exit_terms_reference;
-- Customer API addresses come from the official host or a verified domain.
-- The stored brand URL was no longer written or read.
ALTER TABLE organization_brands DROP COLUMN api_base_url;
