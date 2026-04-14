-- name: SetConfigEntry :one
INSERT INTO config_entries (id, namespace, key, value, updated_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (namespace, key) DO UPDATE SET
    value      = excluded.value,
    updated_at = excluded.updated_at
RETURNING *;

-- name: GetConfigEntry :one
SELECT * FROM config_entries WHERE namespace = $1 AND key = $2;

-- name: ListConfigByNamespace :many
SELECT * FROM config_entries WHERE namespace = $1 ORDER BY key ASC;

-- name: ListAllConfig :many
SELECT * FROM config_entries ORDER BY namespace ASC, key ASC;

-- name: ListConfigNamespaces :many
SELECT DISTINCT namespace FROM config_entries ORDER BY namespace ASC;

-- name: DeleteConfigEntry :exec
DELETE FROM config_entries WHERE namespace = $1 AND key = $2;

-- name: DeleteConfigNamespace :exec
DELETE FROM config_entries WHERE namespace = $1;

-- Subscriptions

-- name: Subscribe :exec
INSERT INTO config_subscriptions (id, service_id, namespace)
VALUES ($1, $2, $3)
ON CONFLICT DO NOTHING;

-- name: Unsubscribe :exec
DELETE FROM config_subscriptions WHERE service_id = $1 AND namespace = $2;

-- name: ListSubscriptionsByService :many
SELECT namespace FROM config_subscriptions WHERE service_id = $1;

-- name: ListSubscribersByNamespace :many
SELECT s.* FROM services s
JOIN config_subscriptions cs ON s.id = cs.service_id
WHERE cs.namespace = $1
ORDER BY s.name ASC;

-- name: DeleteSubscriptionsByService :exec
DELETE FROM config_subscriptions WHERE service_id = $1;
