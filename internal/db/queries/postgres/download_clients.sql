-- name: CreateDownloadClient :one
INSERT INTO download_clients (id, name, kind, protocol, enabled, priority, host, port, use_ssl, username, password, category, directory, settings, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
RETURNING *;

-- name: GetDownloadClient :one
SELECT * FROM download_clients WHERE id = $1;

-- name: ListDownloadClients :many
SELECT * FROM download_clients ORDER BY priority ASC, name ASC;

-- name: ListEnabledDownloadClients :many
SELECT * FROM download_clients WHERE enabled = TRUE ORDER BY priority ASC, name ASC;

-- name: ListDownloadClientsByProtocol :many
SELECT * FROM download_clients WHERE protocol = $1 ORDER BY priority ASC, name ASC;

-- name: UpdateDownloadClient :one
UPDATE download_clients SET
    name       = $1,
    kind       = $2,
    protocol   = $3,
    enabled    = $4,
    priority   = $5,
    host       = $6,
    port       = $7,
    use_ssl    = $8,
    username   = $9,
    password   = $10,
    category   = $11,
    directory  = $12,
    settings   = $13,
    updated_at = $14
WHERE id = $15
RETURNING *;

-- name: DeleteDownloadClient :exec
DELETE FROM download_clients WHERE id = $1;
