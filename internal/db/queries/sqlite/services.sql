-- name: CreateService :one
INSERT INTO services (id, name, type, api_url, api_key, health_url, version, status, last_seen, registered, metadata)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetService :one
SELECT * FROM services WHERE id = ?;

-- name: GetServiceByNameAndType :one
SELECT * FROM services WHERE name = ? AND type = ?;

-- name: ListServices :many
SELECT * FROM services ORDER BY type ASC, name ASC;

-- name: ListServicesByType :many
SELECT * FROM services WHERE type = ? ORDER BY name ASC;

-- name: ListOnlineServices :many
SELECT * FROM services WHERE status = 'online' ORDER BY type ASC, name ASC;

-- name: UpdateService :one
UPDATE services SET
    name       = ?,
    api_url    = ?,
    api_key    = ?,
    health_url = ?,
    version    = ?,
    metadata   = ?,
    last_seen  = ?
WHERE id = ?
RETURNING *;

-- name: UpdateServiceStatus :exec
UPDATE services SET status = ?, last_seen = ? WHERE id = ?;

-- name: UpdateServiceHeartbeat :exec
UPDATE services SET last_seen = ?, status = 'online' WHERE id = ?;

-- name: DeleteService :exec
DELETE FROM services WHERE id = ?;

-- name: CountServicesByType :one
SELECT COUNT(*) FROM services WHERE type = ?;

-- Capabilities

-- name: AddCapability :exec
INSERT OR IGNORE INTO service_capabilities (id, service_id, capability)
VALUES (?, ?, ?);

-- name: ListCapabilities :many
SELECT capability FROM service_capabilities WHERE service_id = ?;

-- name: DeleteCapabilities :exec
DELETE FROM service_capabilities WHERE service_id = ?;

-- name: ListServicesByCapability :many
SELECT s.* FROM services s
JOIN service_capabilities sc ON s.id = sc.service_id
WHERE sc.capability = ?
ORDER BY s.type ASC, s.name ASC;
