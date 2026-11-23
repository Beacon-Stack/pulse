-- +goose Up

-- One row per real scrape proxied through the torznab endpoint (cache hits
-- are not scrapes and are not recorded). Feeds the per-indexer speed,
-- result and Cloudflare stats. Rows older than 30 days are pruned.
CREATE TABLE indexer_search_events (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    indexer_id     TEXT    NOT NULL REFERENCES indexers(id) ON DELETE CASCADE,
    at             TEXT    NOT NULL,
    duration_ms    INTEGER NOT NULL,
    ok             BOOLEAN NOT NULL,
    error_kind     TEXT    NOT NULL DEFAULT '',
    results        INTEGER NOT NULL DEFAULT 0,
    with_seeders   INTEGER NOT NULL DEFAULT 0,
    median_seeders INTEGER NOT NULL DEFAULT 0,
    challenged     BOOLEAN NOT NULL DEFAULT FALSE,
    solved         BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE INDEX idx_indexer_search_events_at ON indexer_search_events(at);

-- One row per indexer each time several indexers answered the same query:
-- how many of its results no other indexer returned.
CREATE TABLE indexer_overlap_events (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    indexer_id     TEXT    NOT NULL REFERENCES indexers(id) ON DELETE CASCADE,
    at             TEXT    NOT NULL,
    results        INTEGER NOT NULL,
    unique_results INTEGER NOT NULL,
    compared_with  INTEGER NOT NULL
);
CREATE INDEX idx_indexer_overlap_events_at ON indexer_overlap_events(at);

-- +goose Down
DROP TABLE indexer_overlap_events;
DROP TABLE indexer_search_events;
