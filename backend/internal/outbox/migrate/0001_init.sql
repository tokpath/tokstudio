-- Outbox 属于异步投递模块。其他模块只能通过 Publisher 接口写入事件。
CREATE TABLE IF NOT EXISTS outbox_events (
    id TEXT PRIMARY KEY,
    event_type TEXT NOT NULL,
    aggregate_type TEXT NOT NULL,
    aggregate_id TEXT NOT NULL,
    payload_json JSONB NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    attempts INTEGER NOT NULL DEFAULT 0,
    available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS outbox_events_pending_idx
    ON outbox_events (status, available_at);

CREATE TABLE IF NOT EXISTS outbox_consumptions (
    event_id TEXT PRIMARY KEY,
    consumer TEXT NOT NULL,
    consumed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
