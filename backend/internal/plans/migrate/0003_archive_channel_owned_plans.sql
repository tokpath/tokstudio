UPDATE plans_product_plans
SET status = 'archived', updated_at = NOW()
WHERE owner_type = 'channel'
  AND owner_id IN (SELECT id FROM identity_channel_orgs WHERE type = 'B')
  AND status <> 'archived';
