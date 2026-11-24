-- name: CreateTag :one
INSERT INTO tags (id, name) VALUES (?, ?) RETURNING *;

-- name: ListTags :many
SELECT * FROM tags ORDER BY name ASC;

-- name: UpdateTag :one
UPDATE tags SET name = ? WHERE id = ? RETURNING *;

-- name: DeleteTag :exec
DELETE FROM tags WHERE id = ?;

-- Service tags

-- Indexer tags
-- Counts
-- name: CountServicesForTag :one
SELECT COUNT(*) FROM service_tags WHERE tag_id = ?;

-- name: CountIndexersForTag :one
SELECT COUNT(*) FROM indexer_tags WHERE tag_id = ?;
