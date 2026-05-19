-- +goose Up
CREATE TABLE deliveries (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id        UUID NOT NULL REFERENCES events(id),
    endpoint_id     UUID NOT NULL REFERENCES endpoints(id),

    status          TEXT NOT NULL DEFAULT 'pending',
    attempt_count   INT NOT NULL DEFAULT 0,
    next_retry_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    last_http_status    INT,
    last_error          TEXT,
    last_attempted_at   TIMESTAMPTZ,

    delivered_at    TIMESTAMPTZ,
    dead_lettered_at TIMESTAMPTZ,

    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_delivery_event_endpoint UNIQUE (event_id, endpoint_id),
    CONSTRAINT chk_delivery_status CHECK (
        status IN ('pending', 'delivering', 'delivered', 'dead_lettered')
    ),
    CONSTRAINT chk_attempt_count CHECK (attempt_count >= 0),
    CONSTRAINT chk_last_http_status CHECK (
        last_http_status IS NULL OR last_http_status BETWEEN 100 AND 599
    )
);

CREATE INDEX idx_deliveries_worker
    ON deliveries (next_retry_at ASC, id)
    WHERE status = 'pending';

CREATE INDEX idx_deliveries_endpoint_id ON deliveries (endpoint_id);

CREATE INDEX idx_deliveries_dead_lettered
    ON deliveries (dead_lettered_at DESC)
    WHERE status = 'dead_lettered';

-- +goose Down
DROP INDEX IF EXISTS idx_deliveries_dead_lettered;
DROP INDEX IF EXISTS idx_deliveries_endpoint_id;
DROP INDEX IF EXISTS idx_deliveries_worker;
DROP TABLE IF EXISTS deliveries;
