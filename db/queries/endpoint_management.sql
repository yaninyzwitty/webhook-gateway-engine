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
RETURNING id, name, url, topics, secret, active, max_attempts, timeout_ms, created_at, updated_at;

-- name: GetEndpointByID :one
SELECT id, name, url, topics, secret, active, max_attempts, timeout_ms, created_at, updated_at
FROM endpoints
WHERE id = sqlc.arg('id');

-- name: GetEndpointByName :one
SELECT id, name, url, topics, secret, active, max_attempts, timeout_ms, created_at, updated_at
FROM endpoints
WHERE name = sqlc.arg('name');

-- name: ListActiveEndpointsByTopics :many
SELECT id, name, url, topics, secret, active, max_attempts, timeout_ms, created_at, updated_at
FROM endpoints
WHERE active = TRUE AND topics && sqlc.arg('topics')
ORDER BY id;

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
RETURNING id, name, url, topics, secret, active, max_attempts, timeout_ms, created_at, updated_at;

-- name: DeleteEndpoint :exec
DELETE FROM endpoints WHERE id = sqlc.arg('id');
