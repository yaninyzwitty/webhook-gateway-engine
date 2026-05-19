-- +goose Up
CREATE TABLE events (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    idempotency_key TEXT NOT NULL,
    topic           TEXT NOT NULL,
    payload         JSONB NOT NULL,
    received_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_events_idempotency_key UNIQUE (idempotency_key)
);

CREATE INDEX idx_events_received_at ON events (received_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_events_received_at;
DROP TABLE IF EXISTS events;
