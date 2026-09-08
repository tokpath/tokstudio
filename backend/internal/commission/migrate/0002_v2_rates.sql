ALTER TABLE commission_policies
    ADD COLUMN IF NOT EXISTS indirect_bps INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS total_bps INTEGER NOT NULL DEFAULT 0;

UPDATE commission_policies
SET indirect_bps = CASE WHEN override_bps > 0 THEN override_bps ELSE 500 END,
    total_bps = CASE WHEN cap_bps > 0 THEN cap_bps ELSE 2000 END
WHERE total_bps = 0;

UPDATE commission_policies
SET channel_bps = 0, team_bps = 0
WHERE id = 'plc_m6_default';

CREATE TABLE IF NOT EXISTS commission_marketing_entries (
    id TEXT PRIMARY KEY,
    channel_org_id TEXT NOT NULL,
    kind TEXT NOT NULL,
    status TEXT NOT NULL,
    amount_minor BIGINT NOT NULL,
    usage_event_id TEXT,
    commission_entry_id TEXT,
    source_type TEXT,
    source_id TEXT,
    reversal_of TEXT,
    idempotency_key TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS commission_marketing_channel_idx
    ON commission_marketing_entries (channel_org_id, status);
