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

-- name: ClaimPendingDeliveriesForWorker :many
WITH ready AS (
    SELECT id
    FROM deliveries
    WHERE status = 'pending' AND next_retry_at <= NOW()
    ORDER BY next_retry_at ASC, id ASC
    FOR UPDATE SKIP LOCKED
    LIMIT sqlc.arg('limit')
)
UPDATE deliveries
SET status = 'delivering',
    last_attempted_at = NOW()
FROM ready
WHERE deliveries.id = ready.id
RETURNING deliveries.id, deliveries.event_id, deliveries.endpoint_id, deliveries.status,
          deliveries.attempt_count, deliveries.next_retry_at, deliveries.last_http_status,
          deliveries.last_error, deliveries.last_attempted_at, deliveries.delivered_at,
          deliveries.dead_lettered_at, deliveries.created_at;

-- name: ResetStaleDelivering :exec
UPDATE deliveries
SET status = 'pending',
    next_retry_at = NOW()
WHERE status = 'delivering'
  AND last_attempted_at <= NOW() - (sqlc.arg('stale_after_milliseconds')::bigint * INTERVAL '1 millisecond');

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

-- name: RescheduleDelivery :one
UPDATE deliveries
SET status = 'pending',
    next_retry_at = sqlc.arg('next_retry_at'),
    last_error = sqlc.narg('last_error')
WHERE id = sqlc.arg('id')
RETURNING id, event_id, endpoint_id, status, attempt_count, next_retry_at,
          last_http_status, last_error, last_attempted_at, delivered_at, dead_lettered_at, created_at;

-- name: CountPendingDeliveries :one
SELECT COUNT(*) FROM deliveries WHERE status = 'pending';

-- name: ListDeliveriesByEndpoint :many
SELECT id, event_id, endpoint_id, status, attempt_count, next_retry_at,
       last_http_status, last_error, last_attempted_at, delivered_at, dead_lettered_at, created_at
FROM deliveries
WHERE endpoint_id = sqlc.arg('endpoint_id')
  AND (
      cardinality(sqlc.arg('statuses')::text[]) IS NULL
      OR cardinality(sqlc.arg('statuses')::text[]) = 0
      OR status = ANY(sqlc.arg('statuses')::text[])
  )
  AND (
      sqlc.narg('cursor_id')::uuid IS NULL
      OR id > sqlc.narg('cursor_id')::uuid
  )
ORDER BY id
LIMIT sqlc.arg('limit');

-- name: ListDeadLetteredDeliveries :many
SELECT id, event_id, endpoint_id, status, attempt_count, next_retry_at,
       last_http_status, last_error, last_attempted_at, delivered_at, dead_lettered_at, created_at
FROM deliveries
WHERE status = 'dead_lettered'
  AND (
      sqlc.narg('cursor_dead_lettered_at')::timestamptz IS NULL
      OR sqlc.narg('cursor_id')::uuid IS NULL
      OR (dead_lettered_at, id) < (sqlc.narg('cursor_dead_lettered_at')::timestamptz, sqlc.narg('cursor_id')::uuid)
  )
ORDER BY dead_lettered_at DESC, id DESC
LIMIT sqlc.arg('limit');
