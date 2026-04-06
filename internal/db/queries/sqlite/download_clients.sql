-- name: CreateDownloadClient :one
INSERT INTO download_clients (id, name, kind, protocol, enabled, priority, host, port, use_ssl, username, password, category, directory, settings, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetDownloadClient :one
SELECT * FROM download_clients WHERE id = ?;

-- name: ListDownloadClients :many
SELECT * FROM download_clients ORDER BY priority ASC, name ASC;

-- name: ListEnabledDownloadClients :many
SELECT * FROM download_clients WHERE enabled = 1 ORDER BY priority ASC, name ASC;

-- name: ListDownloadClientsByProtocol :many
SELECT * FROM download_clients WHERE protocol = ? ORDER BY priority ASC, name ASC;

-- name: UpdateDownloadClient :one
UPDATE download_clients SET
    name       = ?,
    kind       = ?,
    protocol   = ?,
    enabled    = ?,
    priority   = ?,
    host       = ?,
    port       = ?,
    use_ssl    = ?,
    username   = ?,
    password   = ?,
    category   = ?,
    directory  = ?,
    settings   = ?,
    updated_at = ?
WHERE id = ?
RETURNING *;

-- name: DeleteDownloadClient :exec
DELETE FROM download_clients WHERE id = ?;
