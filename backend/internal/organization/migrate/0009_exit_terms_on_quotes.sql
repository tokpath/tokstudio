-- Transaction-specific exit rights remain on accepted quotes, orders and lots.
ALTER TABLE organization_supply_agreements DROP COLUMN exit_policy;
