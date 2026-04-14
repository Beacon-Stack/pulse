-- name: CreateService :one
INSERT INTO services (id, name, type, api_url, api_key, health_url, version, status, last_seen, registered, metadata)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: GetService :one
SELECT * FROM services WHERE id = $1;

-- name: GetServiceByNameAndType :one
SELECT * FROM services WHERE name = $1 AND type = $2;

-- name: ListServices :many
SELECT * FROM services ORDER BY type ASC, name ASC;

-- name: ListServicesByType :many
SELECT * FROM services WHERE type = $1 ORDER BY name ASC;

-- name: ListOnlineServices :many
SELECT * FROM services WHERE status = 'online' ORDER BY type ASC, name ASC;

-- name: UpdateService :one
UPDATE services SET
    name       = $1,
    api_url    = $2,
    api_key    = $3,
    health_url = $4,
    version    = $5,
    metadata   = $6,
    last_seen  = $7
WHERE id = $8
RETURNING *;

-- name: UpdateServiceStatus :exec
UPDATE services SET status = $1, last_seen = $2 WHERE id = $3;

-- name: UpdateServiceHeartbeat :exec
UPDATE services SET last_seen = $1, status = 'online' WHERE id = $2;

-- name: DeleteService :exec
DELETE FROM services WHERE id = $1;

-- name: CountServicesByType :one
SELECT COUNT(*) FROM services WHERE type = $1;

-- Capabilities

-- name: AddCapability :exec
INSERT INTO service_capabilities (id, service_id, capability)
VALUES ($1, $2, $3)
ON CONFLICT DO NOTHING;

-- name: ListCapabilities :many
SELECT capability FROM service_capabilities WHERE service_id = $1;

-- name: DeleteCapabilities :exec
DELETE FROM service_capabilities WHERE service_id = $1;

-- name: ListServicesByCapability :many
SELECT s.* FROM services s
JOIN service_capabilities sc ON s.id = sc.service_id
WHERE sc.capability = $1
ORDER BY s.type ASC, s.name ASC;
