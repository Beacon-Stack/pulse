-- +goose Up
CREATE TABLE download_clients (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    kind        TEXT NOT NULL,           -- qbittorrent, deluge, transmission, sabnzbd, nzbget
    protocol    TEXT NOT NULL,           -- torrent, usenet
    enabled     INTEGER NOT NULL DEFAULT 1,
    priority    INTEGER NOT NULL DEFAULT 1,
    host        TEXT NOT NULL,
    port        INTEGER NOT NULL,
    use_ssl     INTEGER NOT NULL DEFAULT 0,
    username    TEXT NOT NULL DEFAULT '',
    password    TEXT NOT NULL DEFAULT '',
    category    TEXT NOT NULL DEFAULT '',
    directory   TEXT NOT NULL DEFAULT '',
    settings    TEXT NOT NULL DEFAULT '{}',
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS download_clients;
