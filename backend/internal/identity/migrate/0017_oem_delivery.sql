CREATE TABLE identity_oem_deliveries (
 channel_org_id TEXT PRIMARY KEY REFERENCES identity_channel_orgs(id),
 operation_id TEXT NOT NULL UNIQUE,
 created_by TEXT NOT NULL REFERENCES identity_users(id),
 input_json JSONB NOT NULL,
 sales_mode TEXT NOT NULL DEFAULT 'offline' CHECK (sales_mode IN ('offline','online')),
 sell_plans BOOLEAN NOT NULL DEFAULT false,
 phase TEXT NOT NULL DEFAULT 'configuring' CHECK (phase IN ('configuring','awaiting_acceptance','handed_over','paused')),
 version BIGINT NOT NULL DEFAULT 1,
 domain_evidence_json JSONB NOT NULL DEFAULT '{}',
 receiver_user_id TEXT REFERENCES identity_users(id),
 handed_over_at TIMESTAMPTZ,
 handoff_evidence_json JSONB NOT NULL DEFAULT '{}',
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
