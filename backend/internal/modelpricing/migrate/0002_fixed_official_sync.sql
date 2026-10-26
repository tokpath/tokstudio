-- Official supplier cost reference is a system service, not an operator policy.
ALTER TABLE model_pricing_settings DROP COLUMN enabled, DROP COLUMN source_url, DROP COLUMN interval_hours;
UPDATE model_pricing_settings SET sync_requested = TRUE WHERE id = 'platform';
