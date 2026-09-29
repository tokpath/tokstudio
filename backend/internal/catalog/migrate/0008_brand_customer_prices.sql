DO $$
BEGIN
  IF to_regclass('identity_channel_orgs') IS NOT NULL THEN
    UPDATE catalog_channel_model_policies
    SET customer_override_json = '{}'::jsonb
    WHERE channel_org_id IN (SELECT id FROM identity_channel_orgs WHERE type = 'B');
  END IF;
END $$;
