-- +goose Up

-- ── Shared Media Handling ───────────────────────────────────────────────────
-- Filesystem/handling settings that apply uniformly across all media-manager
-- services (Prism, Pilot). Single-row table; the check constraint enforces it.
-- Fields here are the intersection of what's genuinely shareable — naming
-- templates themselves are per-service because the tokens differ by media type.
CREATE TABLE shared_media_handling (
    id                    INTEGER PRIMARY KEY CHECK (id = 1),
    colon_replacement     TEXT NOT NULL DEFAULT 'space-dash',
    import_extra_files    BOOLEAN NOT NULL DEFAULT FALSE,
    extra_file_extensions TEXT NOT NULL DEFAULT 'srt,nfo',
    rename_files          BOOLEAN NOT NULL DEFAULT TRUE,
    updated_at            TEXT NOT NULL
);

INSERT INTO shared_media_handling (id, updated_at)
VALUES (1, to_char(now() AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'));

-- +goose Down

DROP TABLE IF EXISTS shared_media_handling;
