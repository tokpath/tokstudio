-- Supplier, buyer and exit terms are fixed when the operator is created.
-- A draft supply status blocked purchasing and had no operator action to leave it.
UPDATE organization_supply_agreements SET status='active' WHERE status='draft';
ALTER TABLE organization_supply_agreements DROP CONSTRAINT organization_supply_agreements_status_check;
ALTER TABLE organization_supply_agreements ADD CHECK (status IN ('active','paused','exited'));
