-- +goose Up
CREATE TABLE endpoints (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT NOT NULL,
    url             TEXT NOT NULL,
    topics          TEXT[] NOT NULL,
    secret          TEXT,
    active          BOOLEAN NOT NULL DEFAULT TRUE,

    max_attempts    INT NOT NULL DEFAULT 10,
    timeout_ms      INT NOT NULL DEFAULT 5000,

    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_endpoints_name UNIQUE (name),
    CONSTRAINT chk_endpoint_topics_not_empty CHECK (cardinality(topics) > 0),
    CONSTRAINT chk_max_attempts CHECK (max_attempts BETWEEN 1 AND 20),
    CONSTRAINT chk_timeout CHECK (timeout_ms BETWEEN 100 AND 30000)
);

CREATE INDEX idx_endpoints_active_topics
    ON endpoints USING GIN (topics)
    WHERE active = TRUE;

-- +goose Down
DROP INDEX IF EXISTS idx_endpoints_active_topics;
DROP TABLE IF EXISTS endpoints;
