-- Appearance retains its current values, editor and time. Domain activation
-- history and all financial/business facts remain intact.
ALTER TABLE branding_changes DISABLE TRIGGER branding_audit_immutable;
DELETE FROM branding_changes WHERE action IN ('theme.changed','logo.uploaded');
ALTER TABLE branding_changes ENABLE TRIGGER branding_audit_immutable;
UPDATE branding_theme_configs SET values_json=values_json-'support_url' WHERE values_json ? 'support_url';
