-- name: CreateIndexer :one
INSERT INTO indexers (id, name, kind, enabled, priority, url, api_key, settings, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetIndexer :one
SELECT * FROM indexers WHERE id = ?;

-- name: ListIndexers :many
SELECT * FROM indexers ORDER BY priority ASC, name ASC;

-- name: ListEnabledIndexers :many
SELECT * FROM indexers WHERE enabled = TRUE ORDER BY priority ASC, name ASC;

-- name: UpdateIndexer :one
UPDATE indexers SET
    name       = ?,
    kind       = ?,
    enabled    = ?,
    priority   = ?,
    url        = ?,
    api_key    = ?,
    settings   = ?,
    updated_at = ?
WHERE id = ?
RETURNING *;

-- name: DeleteIndexer :exec
DELETE FROM indexers WHERE id = ?;

-- name: SetIndexerHealth :exec
UPDATE indexers SET
    health_status     = ?,
    health_message    = ?,
    health_checked_at = ?
WHERE id = ?;

-- Assignments

-- name: CreateAssignment :one
INSERT INTO indexer_assignments (id, indexer_id, service_id, overrides)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: ListAssignmentsByIndexer :many
SELECT * FROM indexer_assignments WHERE indexer_id = ?;

-- name: ListAssignmentsByService :many
SELECT * FROM indexer_assignments WHERE service_id = ?;

-- name: DeleteAssignment :exec
DELETE FROM indexer_assignments WHERE indexer_id = ? AND service_id = ?;

-- name: DeleteAssignmentsByIndexer :exec
DELETE FROM indexer_assignments WHERE indexer_id = ?;

-- name: DeleteAssignmentsByService :exec
DELETE FROM indexer_assignments WHERE service_id = ?;

-- name: ListIndexersForService :many
-- Indexers whose catalog definition vanished can only fail searches, so
-- they are withheld from services entirely; the service's next sync then
-- drops its local copy.
SELECT i.* FROM indexers i
JOIN indexer_assignments ia ON i.id = ia.indexer_id
WHERE ia.service_id = ?
  AND i.health_status != 'definition_missing'
ORDER BY i.priority ASC, i.name ASC;

-- name: GetAssignmentOverrides :one
SELECT overrides FROM indexer_assignments WHERE indexer_id = ? AND service_id = ?;
