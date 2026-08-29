-- commission 模块自有表。billing 禁止直连。
CREATE TABLE IF NOT EXISTS commission_policies (
    id TEXT PRIMARY KEY,
    scope_type TEXT NOT NULL,
    scope_id TEXT NOT NULL,
    version TEXT NOT NULL,
    direct_bps INTEGER NOT NULL,
    override_bps INTEGER NOT NULL,
    channel_bps INTEGER NOT NULL,
    team_bps INTEGER NOT NULL,
    cap_bps INTEGER NOT NULL,
    freeze_days INTEGER NOT NULL DEFAULT 7,
    min_settle_minor BIGINT NOT NULL DEFAULT 1000000,
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS commission_entries (
    id TEXT PRIMARY KEY,
    usage_event_id TEXT NOT NULL,
    request_id TEXT,
    user_id TEXT,
    channel_org_id TEXT,
    beneficiary_role_id TEXT,
    kind TEXT NOT NULL,
    policy_version TEXT NOT NULL,
    base_amount_minor BIGINT NOT NULL,
    raw_amount_minor BIGINT NOT NULL,
    amount_minor BIGINT NOT NULL,
    status TEXT NOT NULL,
    available_at TIMESTAMPTZ,
    settlement_id TEXT,
    source_entry_id TEXT,
    reversal_of TEXT,
    idempotency_key TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS commission_settlements (
    id TEXT PRIMARY KEY,
    period_start TIMESTAMPTZ NOT NULL,
    period_end TIMESTAMPTZ NOT NULL,
    channel_org_id TEXT,
    beneficiary_role_id TEXT,
    amount_minor BIGINT NOT NULL,
    status TEXT NOT NULL,
    policy_version TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS commission_payouts (
    id TEXT PRIMARY KEY,
    settlement_id TEXT NOT NULL REFERENCES commission_settlements(id),
    method TEXT NOT NULL,
    reference TEXT,
    actor_user_id TEXT,
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS commission_entries_usage_idx ON commission_entries (usage_event_id, status);
CREATE INDEX IF NOT EXISTS commission_entries_avail_idx ON commission_entries (status, available_at);
CREATE INDEX IF NOT EXISTS commission_entries_bene_idx ON commission_entries (beneficiary_role_id, status);
