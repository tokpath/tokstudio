-- Read-only workflow indexes; historical amounts, owners and snapshots stay intact.
CREATE INDEX IF NOT EXISTS billing_usage_user_page ON billing_usage_events(user_id,occurred_at DESC,id DESC);
CREATE INDEX IF NOT EXISTS billing_usage_channel_page ON billing_usage_events(channel_org_id,occurred_at DESC,id DESC);
CREATE INDEX IF NOT EXISTS billing_usage_pending_page ON billing_usage_events(state,occurred_at DESC,id DESC);
