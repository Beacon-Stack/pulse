-- name: SetConfigEntry :one
INSERT INTO config_entries (id, namespace, key, value, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT (namespace, key) DO UPDATE SET
    value      = excluded.value,
    updated_at = excluded.updated_at
RETURNING *;

-- name: GetConfigEntry :one
SELECT * FROM config_entries WHERE namespace = ? AND key = ?;

-- name: ListConfigByNamespace :many
SELECT * FROM config_entries WHERE namespace = ? ORDER BY key ASC;

-- name: ListAllConfig :many
SELECT * FROM config_entries ORDER BY namespace ASC, key ASC;

-- name: ListConfigNamespaces :many
SELECT DISTINCT namespace FROM config_entries ORDER BY namespace ASC;

-- name: DeleteConfigEntry :exec
DELETE FROM config_entries WHERE namespace = ? AND key = ?;

-- name: DeleteConfigNamespace :exec
DELETE FROM config_entries WHERE namespace = ?;

-- Subscriptions

-- name: Subscribe :exec
INSERT OR IGNORE INTO config_subscriptions (id, service_id, namespace)
VALUES (?, ?, ?);

-- name: Unsubscribe :exec
DELETE FROM config_subscriptions WHERE service_id = ? AND namespace = ?;

-- name: ListSubscriptionsByService :many
SELECT namespace FROM config_subscriptions WHERE service_id = ?;

-- name: ListSubscribersByNamespace :many
SELECT s.* FROM services s
JOIN config_subscriptions cs ON s.id = cs.service_id
WHERE cs.namespace = ?
ORDER BY s.name ASC;

-- name: DeleteSubscriptionsByService :exec
DELETE FROM config_subscriptions WHERE service_id = ?;
