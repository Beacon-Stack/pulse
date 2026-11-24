-- name: ListFilterPresets :many
SELECT * FROM filter_presets ORDER BY name ASC;

-- name: UpsertFilterPreset :one
INSERT INTO filter_presets (id, name, filters, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT (name) DO UPDATE SET
    filters = excluded.filters,
    updated_at = excluded.updated_at
RETURNING *;

-- name: DeleteFilterPreset :exec
DELETE FROM filter_presets WHERE id = ?;

