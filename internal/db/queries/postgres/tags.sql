-- name: CreateTag :one
INSERT INTO tags (id, name) VALUES ($1, $2) RETURNING *;

-- name: GetTag :one
SELECT * FROM tags WHERE id = $1;

-- name: GetTagByName :one
SELECT * FROM tags WHERE name = $1;

-- name: ListTags :many
SELECT * FROM tags ORDER BY name ASC;

-- name: UpdateTag :one
UPDATE tags SET name = $1 WHERE id = $2 RETURNING *;

-- name: DeleteTag :exec
DELETE FROM tags WHERE id = $1;

-- Service tags

-- name: SetServiceTags :exec
DELETE FROM service_tags WHERE service_id = $1;

-- name: AddServiceTag :exec
INSERT INTO service_tags (service_id, tag_id) VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: ListServiceTagIDs :many
SELECT tag_id FROM service_tags WHERE service_id = $1;

-- Indexer tags

-- name: SetIndexerTags :exec
DELETE FROM indexer_tags WHERE indexer_id = $1;

-- name: AddIndexerTag :exec
INSERT INTO indexer_tags (indexer_id, tag_id) VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: ListIndexerTagIDs :many
SELECT tag_id FROM indexer_tags WHERE indexer_id = $1;

-- Counts

-- name: CountServicesForTag :one
SELECT COUNT(*) FROM service_tags WHERE tag_id = $1;

-- name: CountIndexersForTag :one
SELECT COUNT(*) FROM indexer_tags WHERE tag_id = $1;
