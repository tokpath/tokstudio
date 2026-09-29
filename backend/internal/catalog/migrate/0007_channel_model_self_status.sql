ALTER TABLE catalog_channel_model_policies
    ADD COLUMN IF NOT EXISTS self_enabled BOOLEAN NOT NULL DEFAULT true;

-- Legacy C->B grants were copied automatically on channel creation. The new
-- delegation flow starts these child channels without an implicit grant.
DO $$
BEGIN
  IF to_regclass('identity_channel_orgs') IS NOT NULL THEN
    DELETE FROM catalog_channel_model_policies p
    USING identity_channel_orgs child, identity_channel_orgs parent
    WHERE p.channel_org_id = child.id
      AND child.parent_id = parent.id
      AND parent.type = 'C';
  END IF;
END $$;
