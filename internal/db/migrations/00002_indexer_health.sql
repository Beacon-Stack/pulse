-- +goose Up

-- Indexer catalog health, maintained by Manager.AuditCatalog after every
-- Prowlarr definition load. 'definition_missing' means the indexer's
-- Cardigann definition no longer resolves — searches through it can only
-- fail, so it is also withheld from services (see ListIndexersForService).
ALTER TABLE indexers ADD COLUMN health_status TEXT NOT NULL DEFAULT 'unknown';
ALTER TABLE indexers ADD COLUMN health_message TEXT NOT NULL DEFAULT '';
ALTER TABLE indexers ADD COLUMN health_checked_at TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE indexers DROP COLUMN health_status;
ALTER TABLE indexers DROP COLUMN health_message;
ALTER TABLE indexers DROP COLUMN health_checked_at;
