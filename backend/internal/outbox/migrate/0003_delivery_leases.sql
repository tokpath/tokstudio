ALTER TABLE outbox_events ADD COLUMN lease_until TIMESTAMPTZ;
ALTER TABLE outbox_events ADD COLUMN fence BIGINT NOT NULL DEFAULT 0;
ALTER TABLE outbox_events ADD COLUMN max_attempts INTEGER NOT NULL DEFAULT 12 CHECK (max_attempts BETWEEN 1 AND 100);
ALTER TABLE outbox_events ADD CONSTRAINT outbox_delivery_status CHECK (status IN ('pending','publishing','published','failed'));
CREATE INDEX outbox_events_lease_idx ON outbox_events(lease_until) WHERE status='publishing';
-- NULL explicitly means replay all existing events. started_at is registration
-- metadata only; it must never hide previously committed business obligations.
ALTER TABLE outbox_consumers ADD COLUMN replay_from TIMESTAMPTZ;
