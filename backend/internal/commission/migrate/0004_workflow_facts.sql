-- Workflow identities are internal records, never fabricated external proof.
CREATE TABLE commission_workflow_previews (
    id TEXT PRIMARY KEY,
    owner_id TEXT NOT NULL,
    actor_user_id TEXT NOT NULL,
    channels_json JSONB NOT NULL,
    ignore_minimum BOOLEAN NOT NULL,
    fingerprint TEXT NOT NULL,
    preview_json JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE commission_workflow_operations (
    id TEXT PRIMARY KEY,
    owner_id TEXT NOT NULL,
    actor_user_id TEXT NOT NULL,
    operation_id TEXT NOT NULL,
    kind TEXT NOT NULL,
    payload_hash TEXT NOT NULL,
    channels_json JSONB NOT NULL,
    result_json JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(owner_id, actor_user_id, operation_id)
);
ALTER TABLE commission_payouts ADD COLUMN occurred_at TIMESTAMPTZ,
    ADD COLUMN note TEXT NOT NULL DEFAULT '',
    ADD COLUMN scope_id TEXT NOT NULL DEFAULT '';
ALTER TABLE commission_settlements ADD COLUMN entry_ids_json JSONB;
-- Preserve unknown historical payment dates. Registration time is not proof of
-- the actual payment time, so do not backfill occurred_at from created_at.
UPDATE commission_payouts SET scope_id='legacy:' || settlement_id;
CREATE UNIQUE INDEX commission_payout_reference_scope_idx
    ON commission_payouts(scope_id, reference) WHERE reference IS NOT NULL AND reference <> '';
CREATE UNIQUE INDEX commission_payout_one_settlement_idx ON commission_payouts(settlement_id);
