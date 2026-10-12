-- Payment retries are independent of access validity. Keep the old nullable
-- column for rolling upgrades, but no new API or state exposes a grace date.
UPDATE plans_subscriptions SET grace_until = NULL WHERE grace_until IS NOT NULL;
UPDATE plans_subscriptions SET next_retry_at = NULL
WHERE status = 'past_due' AND (renewal_policy <> 'auto' OR payment_adapter IS DISTINCT FROM 'stripe' OR retry_count >= 4);

CREATE TABLE plans_notification_marks (
    subscription_id TEXT NOT NULL REFERENCES plans_subscriptions(id),
    period_end TIMESTAMPTZ NOT NULL,
    kind TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (subscription_id, period_end, kind)
);
