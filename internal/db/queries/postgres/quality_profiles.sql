-- name: CreateQualityProfile :one
INSERT INTO quality_profiles (
    id, name, cutoff_json, qualities_json,
    upgrade_allowed, upgrade_until_json,
    min_custom_format_score, upgrade_until_cf_score,
    created_at, updated_at
) VALUES (
    $1, $2, $3, $4,
    $5, $6,
    $7, $8,
    $9, $10
)
RETURNING *;

-- name: GetQualityProfile :one
SELECT * FROM quality_profiles WHERE id = $1;

-- name: ListQualityProfiles :many
SELECT * FROM quality_profiles ORDER BY name ASC;

-- name: UpdateQualityProfile :one
UPDATE quality_profiles SET
    name                    = $1,
    cutoff_json             = $2,
    qualities_json          = $3,
    upgrade_allowed         = $4,
    upgrade_until_json      = $5,
    min_custom_format_score = $6,
    upgrade_until_cf_score  = $7,
    updated_at              = $8
WHERE id = $9
RETURNING *;

-- name: DeleteQualityProfile :exec
DELETE FROM quality_profiles WHERE id = $1;
