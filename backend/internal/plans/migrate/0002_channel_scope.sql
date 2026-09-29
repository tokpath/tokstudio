ALTER TABLE plans_product_plans ADD COLUMN IF NOT EXISTS channel_scope TEXT NOT NULL DEFAULT 'all';

CREATE TABLE IF NOT EXISTS plans_plan_channels (
    plan_id TEXT NOT NULL REFERENCES plans_product_plans(id) ON DELETE CASCADE,
    channel_id TEXT NOT NULL,
    PRIMARY KEY (plan_id, channel_id)
);
CREATE INDEX IF NOT EXISTS plans_plan_channels_channel_idx ON plans_plan_channels (channel_id);

UPDATE plans_plan_items AS item
SET expires_in_seconds = 0
FROM plans_product_plans AS plan
WHERE item.plan_id = plan.id AND plan.billing_period = 'once';

UPDATE plans_entitlement_accounts AS entitlement
SET expires_at = NULL
FROM plans_subscriptions AS subscription, plans_product_plans AS plan
WHERE entitlement.source_type = 'plan'
  AND entitlement.source_id = subscription.id
  AND subscription.plan_id = plan.id
  AND plan.billing_period = 'once'
  AND entitlement.status = 'active';
