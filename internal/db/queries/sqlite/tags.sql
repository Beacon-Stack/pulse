-- name: CreateTag :one
INSERT INTO tags (id, name) VALUES (?, ?) RETURNING *;

-- name: GetTag :one
SELECT * FROM tags WHERE id = ?;

-- name: GetTagByName :one
SELECT * FROM tags WHERE name = ?;

-- name: ListTags :many
SELECT * FROM tags ORDER BY name ASC;

-- name: UpdateTag :one
UPDATE tags SET name = ? WHERE id = ? RETURNING *;

-- name: DeleteTag :exec
DELETE FROM tags WHERE id = ?;

-- Service tags

-- name: SetServiceTags :exec
DELETE FROM service_tags WHERE service_id = ?;

-- name: AddServiceTag :exec
INSERT INTO service_tags (service_id, tag_id) VALUES (?, ?)
ON CONFLICT DO NOTHING;

-- name: ListServiceTagIDs :many
SELECT tag_id FROM service_tags WHERE service_id = ?;

-- Indexer tags

-- name: SetIndexerTags :exec
DELETE FROM indexer_tags WHERE indexer_id = ?;

-- name: AddIndexerTag :exec
INSERT INTO indexer_tags (indexer_id, tag_id) VALUES (?, ?)
ON CONFLICT DO NOTHING;

-- name: ListIndexerTagIDs :many
SELECT tag_id FROM indexer_tags WHERE indexer_id = ?;

-- Counts

-- name: CountServicesForTag :one
SELECT COUNT(*) FROM service_tags WHERE tag_id = ?;

-- name: CountIndexersForTag :one
SELECT COUNT(*) FROM indexer_tags WHERE tag_id = ?;
