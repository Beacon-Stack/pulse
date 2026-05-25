-- name: CreateQualityProfile :one
INSERT INTO quality_profiles (
    id, name, cutoff_json, qualities_json,
    upgrade_allowed, upgrade_until_json,
    min_custom_format_score, upgrade_until_cf_score,
    created_at, updated_at
) VALUES (
    ?, ?, ?, ?,
    ?, ?,
    ?, ?,
    ?, ?
)
RETURNING *;

-- name: GetQualityProfile :one
SELECT * FROM quality_profiles WHERE id = ?;

-- name: ListQualityProfiles :many
SELECT * FROM quality_profiles ORDER BY name ASC;

-- name: UpdateQualityProfile :one
UPDATE quality_profiles SET
    name                    = ?,
    cutoff_json             = ?,
    qualities_json          = ?,
    upgrade_allowed         = ?,
    upgrade_until_json      = ?,
    min_custom_format_score = ?,
    upgrade_until_cf_score  = ?,
    updated_at              = ?
WHERE id = ?
RETURNING *;

-- name: DeleteQualityProfile :exec
DELETE FROM quality_profiles WHERE id = ?;
