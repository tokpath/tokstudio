CREATE TABLE incentives_event_dispute_holders (
 event_id UUID NOT NULL REFERENCES incentives_confirmation_events(id),reference TEXT NOT NULL,
 active BOOLEAN NOT NULL,evidence TEXT NOT NULL,created_at TIMESTAMPTZ NOT NULL,updated_at TIMESTAMPTZ NOT NULL,
 PRIMARY KEY(event_id,reference)
);
