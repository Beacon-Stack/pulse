-- +goose Up

-- Search-outcome tracking, maintained by Manager.RecordSearchOutcome on
-- every proxied torznab search. failure_count counts consecutive
-- failures; reaching the threshold flips health_status to 'failing'
-- (a live, recoverable state — unlike definition_missing it is NOT
-- withheld from services).
ALTER TABLE indexers ADD COLUMN failure_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE indexers ADD COLUMN last_error TEXT NOT NULL DEFAULT '';
ALTER TABLE indexers ADD COLUMN last_success_at TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE indexers DROP COLUMN failure_count;
ALTER TABLE indexers DROP COLUMN last_error;
ALTER TABLE indexers DROP COLUMN last_success_at;
