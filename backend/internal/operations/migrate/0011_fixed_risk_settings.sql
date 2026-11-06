-- Prelaunch: the old arbitrary rules (including escalation and technical
-- metrics) do not match the two fixed replenishment settings. Recreate only
-- these settings and their generated notices; no assets or other facts change.
DELETE FROM operations_deliveries WHERE notification_id IN
 (SELECT id FROM operations_notifications WHERE reference LIKE 'risk:%');
DELETE FROM operations_notifications WHERE reference LIKE 'risk:%';
DELETE FROM operations_risk_states;
DELETE FROM operations_risk_rules;
ALTER TABLE operations_risk_rules ADD CONSTRAINT operations_risk_metric_unique UNIQUE(operator_id,metric);
ALTER TABLE operations_risk_rules ADD CONSTRAINT operations_risk_fixed_metric CHECK(metric IN ('sales_space_atg','procurement_available_atg'));
ALTER TABLE operations_risk_rules ADD CONSTRAINT operations_risk_fixed_threshold CHECK(warning>0 AND critical=0 AND direction='below');
