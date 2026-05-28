-- name: CreateEndpoint :one
INSERT INTO endpoints (name, url, topics, secret, active, max_attempts, timeout_ms)
VALUES (
    sqlc.arg('name'),
    sqlc.arg('url'),
    sqlc.arg('topics'),
    sqlc.narg('secret'),
    sqlc.arg('active'),
    sqlc.arg('max_attempts'),
    sqlc.arg('timeout_ms')
)
RETURNING id, name, url, topics, secret, active, max_attempts, timeout_ms,
          circuit_state, failure_count, last_failure_at, cooldown_until,
          created_at, updated_at;

-- name: GetEndpointByID :one
SELECT id, name, url, topics, secret, active, max_attempts, timeout_ms,
       circuit_state, failure_count, last_failure_at, cooldown_until,
       created_at, updated_at
FROM endpoints
WHERE id = sqlc.arg('id');

-- name: GetEndpointByName :one
SELECT id, name, url, topics, secret, active, max_attempts, timeout_ms,
       circuit_state, failure_count, last_failure_at, cooldown_until,
       created_at, updated_at
FROM endpoints
WHERE name = sqlc.arg('name');

-- name: ListActiveEndpointsByTopics :many
SELECT id, name, url, topics, secret, active, max_attempts, timeout_ms,
       circuit_state, failure_count, last_failure_at, cooldown_until,
       created_at, updated_at
FROM endpoints
WHERE active = TRUE AND topics && sqlc.arg('topics')
ORDER BY id;

-- name: ListEndpoints :many
SELECT id, name, url, topics, secret, active, max_attempts, timeout_ms,
       circuit_state, failure_count, last_failure_at, cooldown_until,
       created_at, updated_at
FROM endpoints
WHERE (
    NOT sqlc.arg('active_only')::bool OR active = TRUE
)
AND (
    cardinality(sqlc.arg('topics')::text[]) IS NULL
    OR cardinality(sqlc.arg('topics')::text[]) = 0
    OR topics && sqlc.arg('topics')::text[]
)
AND (
    sqlc.narg('cursor_id')::uuid IS NULL
    OR id > sqlc.narg('cursor_id')::uuid
)
ORDER BY id
LIMIT sqlc.arg('limit');

-- name: UpdateEndpoint :one
UPDATE endpoints
SET name = sqlc.arg('name'),
    url = sqlc.arg('url'),
    topics = sqlc.arg('topics'),
    secret = sqlc.narg('secret'),
    active = sqlc.arg('active'),
    max_attempts = sqlc.arg('max_attempts'),
    timeout_ms = sqlc.arg('timeout_ms'),
    updated_at = NOW()
WHERE id = sqlc.arg('id')
RETURNING id, name, url, topics, secret, active, max_attempts, timeout_ms,
          circuit_state, failure_count, last_failure_at, cooldown_until,
          created_at, updated_at;

-- name: MarkEndpointDeliverySuccess :one
UPDATE endpoints
SET circuit_state = 'closed',
    failure_count = 0,
    last_failure_at = NULL,
    cooldown_until = NULL,
    updated_at = NOW()
WHERE id = sqlc.arg('id')
RETURNING id, name, url, topics, secret, active, max_attempts, timeout_ms,
          circuit_state, failure_count, last_failure_at, cooldown_until,
          created_at, updated_at;

-- name: MarkEndpointDeliveryFailure :one
UPDATE endpoints
SET failure_count = failure_count + 1,
    last_failure_at = NOW(),
    circuit_state = CASE
        WHEN failure_count + 1 >= sqlc.arg('failure_threshold') THEN 'open'
        ELSE circuit_state
    END,
    cooldown_until = CASE
        WHEN failure_count + 1 >= sqlc.arg('failure_threshold') THEN sqlc.arg('cooldown_until')
        ELSE cooldown_until
    END,
    updated_at = NOW()
WHERE id = sqlc.arg('id')
RETURNING id, name, url, topics, secret, active, max_attempts, timeout_ms,
          circuit_state, failure_count, last_failure_at, cooldown_until,
          created_at, updated_at;

-- name: MoveEndpointCircuitToHalfOpen :one
UPDATE endpoints
SET circuit_state = 'half_open',
    updated_at = NOW()
WHERE id = sqlc.arg('id')
  AND circuit_state = 'open'
  AND cooldown_until <= NOW()
RETURNING id, name, url, topics, secret, active, max_attempts, timeout_ms,
          circuit_state, failure_count, last_failure_at, cooldown_until,
          created_at, updated_at;

-- name: DeleteEndpoint :exec
DELETE FROM endpoints WHERE id = sqlc.arg('id');
