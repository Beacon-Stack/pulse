-- name: InsertSearchEvent :exec
INSERT INTO indexer_search_events
    (indexer_id, at, duration_ms, ok, error_kind, results, with_seeders, median_seeders, challenged, solved, query, mode)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

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

-- name: ListRecentQueries :many
-- Distinct recent real queries of one mode, newest first.
SELECT query, MAX(at) AS last_at FROM indexer_search_events
WHERE mode = ? AND query != '' AND ok
GROUP BY query
ORDER BY last_at DESC
LIMIT ?;

-- name: InsertCatalogProbe :one
INSERT INTO catalog_probes
    (catalog_id, cycle_id, at, query, manual, duration_ms, ok, error_kind, error_message,
     results, with_seeders, median_seeders, challenged, solved)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id;

-- name: SetCatalogProbeUnique :exec
UPDATE catalog_probes SET unique_results = ? WHERE id = ?;

-- name: ListCatalogProbesSince :many
SELECT * FROM catalog_probes WHERE at >= ? ORDER BY catalog_id, at;

-- name: PruneCatalogProbes :exec
DELETE FROM catalog_probes WHERE at < ?;
