-- +goose Up

-- What each real search asked for, so the catalog tester can reuse recent
-- real queries (titles from the user's libraries) for its test searches.
ALTER TABLE indexer_search_events ADD COLUMN query TEXT NOT NULL DEFAULT '';
ALTER TABLE indexer_search_events ADD COLUMN mode  TEXT NOT NULL DEFAULT '';

-- One row per test search the catalog tester ran against an indexer that
-- has not been added (keyed by catalog/definition ID). unique_results is
-- filled in when the test cycle ends: how many results no other tested
-- indexer returned for the same query (-1 until then, or if the cycle was
-- cut short). Rows older than 30 days are pruned.
CREATE TABLE catalog_probes (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    catalog_id     TEXT    NOT NULL,
    cycle_id       TEXT    NOT NULL,
    at             TEXT    NOT NULL,
    query          TEXT    NOT NULL,
    manual         BOOLEAN NOT NULL DEFAULT FALSE,
    duration_ms    INTEGER NOT NULL,
    ok             BOOLEAN NOT NULL,
    error_kind     TEXT    NOT NULL DEFAULT '',
    error_message  TEXT    NOT NULL DEFAULT '',
    results        INTEGER NOT NULL DEFAULT 0,
    with_seeders   INTEGER NOT NULL DEFAULT 0,
    median_seeders INTEGER NOT NULL DEFAULT 0,
    unique_results INTEGER NOT NULL DEFAULT -1,
    challenged     BOOLEAN NOT NULL DEFAULT FALSE,
    solved         BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE INDEX idx_catalog_probes_at ON catalog_probes(at);
CREATE INDEX idx_catalog_probes_catalog_at ON catalog_probes(catalog_id, at);

-- +goose Down
DROP TABLE catalog_probes;
ALTER TABLE indexer_search_events DROP COLUMN mode;
ALTER TABLE indexer_search_events DROP COLUMN query;
