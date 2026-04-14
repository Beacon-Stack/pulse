-- name: CreateFilterPreset :one
INSERT INTO filter_presets (id, name, filters, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetFilterPreset :one
SELECT * FROM filter_presets WHERE id = $1;

-- name: GetFilterPresetByName :one
SELECT * FROM filter_presets WHERE name = $1;

-- name: ListFilterPresets :many
SELECT * FROM filter_presets ORDER BY name ASC;

-- name: UpdateFilterPreset :one
UPDATE filter_presets SET name = $1, filters = $2, updated_at = $3
WHERE id = $4
RETURNING *;

-- name: UpsertFilterPreset :one
INSERT INTO filter_presets (id, name, filters, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (name) DO UPDATE SET
    filters = excluded.filters,
    updated_at = excluded.updated_at
RETURNING *;

-- name: DeleteFilterPreset :exec
DELETE FROM filter_presets WHERE id = $1;

-- name: DeleteFilterPresetByName :exec
DELETE FROM filter_presets WHERE name = $1;
