-- billing_period lifetime means the credit does not expire.
-- The previous value once is removed. Rows that still use it are rewritten,
-- then the column rejects every other value.

UPDATE plans_product_plans
SET billing_period = 'lifetime', auto_renew_allowed = false, updated_at = now()
WHERE billing_period = 'once';

UPDATE plans_plan_items AS item
SET expires_in_seconds = 0
FROM plans_product_plans AS plan
WHERE item.plan_id = plan.id AND plan.billing_period = 'lifetime';

UPDATE plans_subscriptions AS subscription
SET current_period_end = NULL, renewal_policy = 'manual', updated_at = now()
FROM plans_product_plans AS plan
WHERE subscription.plan_id = plan.id AND plan.billing_period = 'lifetime';

UPDATE plans_entitlement_accounts AS entitlement
SET expires_at = NULL
FROM plans_subscriptions AS subscription, plans_product_plans AS plan
WHERE entitlement.source_type = 'plan'
  AND entitlement.source_id = subscription.id
  AND subscription.plan_id = plan.id
  AND plan.billing_period = 'lifetime';

ALTER TABLE plans_product_plans DROP CONSTRAINT IF EXISTS plans_product_plans_billing_period_check;
ALTER TABLE plans_product_plans
  ADD CONSTRAINT plans_product_plans_billing_period_check
  CHECK (billing_period IN ('lifetime', 'monthly', 'quarterly', 'yearly'));
