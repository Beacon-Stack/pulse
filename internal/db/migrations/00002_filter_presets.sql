-- +goose Up
CREATE TABLE filter_presets (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL UNIQUE,
    filters    TEXT NOT NULL,  -- JSON: {"protocols":[],"privacies":[],"categories":[],"languages":[],"search":""}
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS filter_presets;
