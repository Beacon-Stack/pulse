-- name: SetConfigEntry :one
INSERT INTO config_entries (id, namespace, key, value, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT (namespace, key) DO UPDATE SET
    value      = excluded.value,
    updated_at = excluded.updated_at
RETURNING *;

-- name: GetConfigEntry :one
SELECT * FROM config_entries WHERE namespace = ? AND key = ?;

-- name: DeleteConfigEntry :exec
DELETE FROM config_entries WHERE namespace = ? AND key = ?;

-- Subscriptions
-- name: Subscribe :exec
INSERT INTO config_subscriptions (id, service_id, namespace)
VALUES (?, ?, ?)
ON CONFLICT DO NOTHING;

-- name: Unsubscribe :exec
DELETE FROM config_subscriptions WHERE service_id = ? AND namespace = ?;

