-- name: CreateFilterPreset :one
INSERT INTO filter_presets (id, name, filters, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: GetFilterPreset :one
SELECT * FROM filter_presets WHERE id = ?;

-- name: GetFilterPresetByName :one
SELECT * FROM filter_presets WHERE name = ?;

-- name: ListFilterPresets :many
SELECT * FROM filter_presets ORDER BY name ASC;

-- name: UpdateFilterPreset :one
UPDATE filter_presets SET name = ?, filters = ?, updated_at = ?
WHERE id = ?
RETURNING *;

-- name: UpsertFilterPreset :one
INSERT INTO filter_presets (id, name, filters, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT (name) DO UPDATE SET
    filters = excluded.filters,
    updated_at = excluded.updated_at
RETURNING *;

-- name: DeleteFilterPreset :exec
DELETE FROM filter_presets WHERE id = ?;

-- name: DeleteFilterPresetByName :exec
DELETE FROM filter_presets WHERE name = ?;
