-- name: CreateEvent :one
INSERT INTO events (idempotency_key, topic, payload)
VALUES (sqlc.arg('idempotency_key'), sqlc.arg('topic'), sqlc.arg('payload'))
RETURNING id, idempotency_key, topic, payload, received_at;

-- name: GetEventByID :one
SELECT id, idempotency_key, topic, payload, received_at
FROM events
WHERE id = sqlc.arg('id');

-- name: GetEventByIdempotencyKey :one
SELECT id, idempotency_key, topic, payload, received_at
FROM events
WHERE idempotency_key = sqlc.arg('idempotency_key');

-- name: ListEvents :many
SELECT id, idempotency_key, topic, payload, received_at
FROM events
WHERE (
    (sqlc.narg('cursor_received_at')::timestamptz IS NULL AND sqlc.narg('cursor_id')::uuid IS NULL)
    OR (received_at, id) < (sqlc.narg('cursor_received_at')::timestamptz, sqlc.narg('cursor_id')::uuid)
)
ORDER BY received_at DESC, id DESC
LIMIT sqlc.arg('limit');
