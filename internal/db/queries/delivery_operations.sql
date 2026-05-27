-- name: CreateDelivery :one
INSERT INTO deliveries (event_id, endpoint_id, status, attempt_count, next_retry_at)
VALUES (sqlc.arg('event_id'), sqlc.arg('endpoint_id'), 'pending', 0, NOW())
RETURNING id, event_id, endpoint_id, status, attempt_count, next_retry_at,
          last_http_status, last_error, last_attempted_at, delivered_at, dead_lettered_at, created_at;

-- name: CreateDeliveriesForEvent :many
INSERT INTO deliveries (event_id, endpoint_id, status, attempt_count, next_retry_at)
SELECT sqlc.arg('event_id'), endpoints.id, 'pending', 0, NOW()
FROM endpoints
WHERE endpoints.active = TRUE
  AND endpoints.topics && sqlc.arg('topics')
ON CONFLICT (event_id, endpoint_id) DO NOTHING
RETURNING id, event_id, endpoint_id, status, attempt_count, next_retry_at,
          last_http_status, last_error, last_attempted_at, delivered_at, dead_lettered_at, created_at;

-- name: GetDeliveryByID :one
SELECT id, event_id, endpoint_id, status, attempt_count, next_retry_at,
       last_http_status, last_error, last_attempted_at, delivered_at, dead_lettered_at, created_at
FROM deliveries
WHERE id = sqlc.arg('id');

-- name: GetDeliveryByEventAndEndpoint :one
SELECT id, event_id, endpoint_id, status, attempt_count, next_retry_at,
       last_http_status, last_error, last_attempted_at, delivered_at, dead_lettered_at, created_at
FROM deliveries
WHERE event_id = sqlc.arg('event_id') AND endpoint_id = sqlc.arg('endpoint_id');

-- name: GetPendingDeliveriesForWorker :many
-- Ready pending deliveries for worker queue processing.
SELECT id, event_id, endpoint_id, status, attempt_count, next_retry_at,
       last_http_status, last_error, last_attempted_at, delivered_at, dead_lettered_at, created_at
FROM deliveries
WHERE status = 'pending' AND next_retry_at <= NOW()
ORDER BY next_retry_at ASC, id ASC
LIMIT sqlc.arg('limit');

-- name: UpdateDeliveryAttempt :one
UPDATE deliveries
SET status = sqlc.arg('status'),
    attempt_count = attempt_count + 1,
    last_http_status = sqlc.narg('last_http_status'),
    last_error = sqlc.narg('last_error'),
    last_attempted_at = NOW(),
    next_retry_at = sqlc.arg('next_retry_at'),
    delivered_at = sqlc.narg('delivered_at'),
    dead_lettered_at = sqlc.narg('dead_lettered_at')
WHERE id = sqlc.arg('id')
RETURNING id, event_id, endpoint_id, status, attempt_count, next_retry_at,
          last_http_status, last_error, last_attempted_at, delivered_at, dead_lettered_at, created_at;

-- name: CountPendingDeliveries :one
SELECT COUNT(*) FROM deliveries WHERE status = 'pending';
