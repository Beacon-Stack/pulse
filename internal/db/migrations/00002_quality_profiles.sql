-- +goose Up

-- ── Quality Profiles ─────────────────────────────────────────────────────────
-- Centralized quality profiles pushed to media-manager services (Prism, Pilot).
-- JSON blobs (cutoff_json, qualities_json, upgrade_until_json) are passed
-- through to services verbatim — Pulse does not interpret them.
CREATE TABLE quality_profiles (
    id                      TEXT PRIMARY KEY,
    name                    TEXT NOT NULL UNIQUE,
    cutoff_json             TEXT NOT NULL DEFAULT '{}',
    qualities_json          TEXT NOT NULL DEFAULT '[]',
    upgrade_allowed         BOOLEAN NOT NULL DEFAULT TRUE,
    upgrade_until_json      TEXT,
    min_custom_format_score INTEGER NOT NULL DEFAULT 0,
    upgrade_until_cf_score  INTEGER NOT NULL DEFAULT 0,
    created_at              TEXT NOT NULL,
    updated_at              TEXT NOT NULL
);

-- +goose Down

DROP TABLE IF EXISTS quality_profiles;
