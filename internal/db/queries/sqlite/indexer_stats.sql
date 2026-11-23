-- name: InsertSearchEvent :exec
INSERT INTO indexer_search_events
    (indexer_id, at, duration_ms, ok, error_kind, results, with_seeders, median_seeders, challenged, solved)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: InsertOverlapEvent :exec
INSERT INTO indexer_overlap_events (indexer_id, at, results, unique_results, compared_with)
VALUES (?, ?, ?, ?, ?);

-- name: ListSearchEventsSince :many
SELECT * FROM indexer_search_events WHERE at >= ? ORDER BY indexer_id, at;

-- name: ListOverlapEventsSince :many
SELECT * FROM indexer_overlap_events WHERE at >= ? ORDER BY indexer_id, at;

-- name: PruneSearchEvents :exec
DELETE FROM indexer_search_events WHERE at < ?;

-- name: PruneOverlapEvents :exec
DELETE FROM indexer_overlap_events WHERE at < ?;
