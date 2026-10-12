CREATE TABLE outbox_consumers (
    name TEXT PRIMARY KEY,
    started_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- 0001 already created consumption receipts with event_id alone as the key.
-- Preserve existing receipts while allowing independent consumers per event.
INSERT INTO outbox_consumers(name,started_at)
SELECT consumer,MIN(consumed_at) FROM outbox_consumptions GROUP BY consumer;
ALTER TABLE outbox_consumptions DROP CONSTRAINT outbox_consumptions_pkey;
ALTER TABLE outbox_consumptions ADD PRIMARY KEY (consumer,event_id);
ALTER TABLE outbox_consumptions ADD FOREIGN KEY (consumer) REFERENCES outbox_consumers(name);
CREATE INDEX outbox_events_subscription_idx ON outbox_events(event_type,created_at,id);
-- Do not turn old historical business logs into a burst of customer mail.
INSERT INTO outbox_consumers(name) VALUES ('notifications-v1') ON CONFLICT DO NOTHING;
