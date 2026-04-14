-- name: CreateIndexer :one
INSERT INTO indexers (id, name, kind, enabled, priority, url, api_key, settings, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: GetIndexer :one
SELECT * FROM indexers WHERE id = $1;

-- name: ListIndexers :many
SELECT * FROM indexers ORDER BY priority ASC, name ASC;

-- name: ListEnabledIndexers :many
SELECT * FROM indexers WHERE enabled = TRUE ORDER BY priority ASC, name ASC;

-- name: UpdateIndexer :one
UPDATE indexers SET
    name       = $1,
    kind       = $2,
    enabled    = $3,
    priority   = $4,
    url        = $5,
    api_key    = $6,
    settings   = $7,
    updated_at = $8
WHERE id = $9
RETURNING *;

-- name: DeleteIndexer :exec
DELETE FROM indexers WHERE id = $1;

-- Assignments

-- name: CreateAssignment :one
INSERT INTO indexer_assignments (id, indexer_id, service_id, overrides)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListAssignmentsByIndexer :many
SELECT * FROM indexer_assignments WHERE indexer_id = $1;

-- name: ListAssignmentsByService :many
SELECT * FROM indexer_assignments WHERE service_id = $1;

-- name: DeleteAssignment :exec
DELETE FROM indexer_assignments WHERE indexer_id = $1 AND service_id = $2;

-- name: DeleteAssignmentsByIndexer :exec
DELETE FROM indexer_assignments WHERE indexer_id = $1;

-- name: DeleteAssignmentsByService :exec
DELETE FROM indexer_assignments WHERE service_id = $1;

-- name: ListIndexersForService :many
SELECT i.* FROM indexers i
JOIN indexer_assignments ia ON i.id = ia.indexer_id
WHERE ia.service_id = $1
ORDER BY i.priority ASC, i.name ASC;

-- name: GetAssignmentOverrides :one
SELECT overrides FROM indexer_assignments WHERE indexer_id = $1 AND service_id = $2;
