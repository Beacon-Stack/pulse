-- +goose Up

-- ── Services ─────────────────────────────────────────────────────────────────
-- Every ecosystem component registers itself here.
CREATE TABLE services (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    type        TEXT NOT NULL,  -- indexer, download-client, media-manager, notification, metadata, automation
    api_url     TEXT NOT NULL,
    api_key     TEXT NOT NULL DEFAULT '',
    health_url  TEXT NOT NULL DEFAULT '',
    version     TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'unknown',  -- online, offline, degraded, unknown
    last_seen   TEXT NOT NULL DEFAULT '',
    registered  TEXT NOT NULL DEFAULT '',
    metadata    TEXT NOT NULL DEFAULT '{}',  -- JSON blob for extra fields
    UNIQUE(name, type)
);

-- ── Service Capabilities ─────────────────────────────────────────────────────
-- Each service declares what it can do.
CREATE TABLE service_capabilities (
    id          TEXT PRIMARY KEY,
    service_id  TEXT NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    capability  TEXT NOT NULL,  -- supports_torrent, supports_usenet, supports_categories, etc.
    UNIQUE(service_id, capability)
);

CREATE INDEX idx_service_capabilities_service ON service_capabilities(service_id);

-- ── Indexers ─────────────────────────────────────────────────────────────────
-- Centrally managed indexer definitions (Prowlarr-like).
CREATE TABLE indexers (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    kind        TEXT NOT NULL DEFAULT 'torznab',  -- torznab, newznab, rss
    enabled     INTEGER NOT NULL DEFAULT 1,
    priority    INTEGER NOT NULL DEFAULT 25,
    url         TEXT NOT NULL,
    api_key     TEXT NOT NULL DEFAULT '',
    settings    TEXT NOT NULL DEFAULT '{}',  -- JSON: extra indexer-specific settings
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

-- ── Indexer Assignments ──────────────────────────────────────────────────────
-- Which services receive which indexers.
CREATE TABLE indexer_assignments (
    id          TEXT PRIMARY KEY,
    indexer_id  TEXT NOT NULL REFERENCES indexers(id) ON DELETE CASCADE,
    service_id  TEXT NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    overrides   TEXT NOT NULL DEFAULT '{}',  -- JSON: per-service override settings
    UNIQUE(indexer_id, service_id)
);

CREATE INDEX idx_indexer_assignments_service ON indexer_assignments(service_id);
CREATE INDEX idx_indexer_assignments_indexer ON indexer_assignments(indexer_id);

-- ── Shared Configuration ─────────────────────────────────────────────────────
-- Key-value config store organized by namespace.
CREATE TABLE config_entries (
    id          TEXT PRIMARY KEY,
    namespace   TEXT NOT NULL,   -- quality, naming, categories, retry, ratelimit, etc.
    key         TEXT NOT NULL,
    value       TEXT NOT NULL,   -- JSON-encoded value
    updated_at  TEXT NOT NULL,
    UNIQUE(namespace, key)
);

CREATE INDEX idx_config_entries_namespace ON config_entries(namespace);

-- ── Config Subscriptions ─────────────────────────────────────────────────────
-- Services subscribe to config namespaces to receive updates.
CREATE TABLE config_subscriptions (
    id          TEXT PRIMARY KEY,
    service_id  TEXT NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    namespace   TEXT NOT NULL,
    UNIQUE(service_id, namespace)
);

CREATE INDEX idx_config_subscriptions_service ON config_subscriptions(service_id);
CREATE INDEX idx_config_subscriptions_namespace ON config_subscriptions(namespace);

-- ── Tags ─────────────────────────────────────────────────────────────────────
CREATE TABLE tags (
    id   TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE
);

-- ── Tag Mappings ─────────────────────────────────────────────────────────────
CREATE TABLE service_tags (
    service_id TEXT NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    tag_id     TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (service_id, tag_id)
);

CREATE TABLE indexer_tags (
    indexer_id TEXT NOT NULL REFERENCES indexers(id) ON DELETE CASCADE,
    tag_id     TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (indexer_id, tag_id)
);

-- +goose Down
DROP TABLE IF EXISTS indexer_tags;
DROP TABLE IF EXISTS service_tags;
DROP TABLE IF EXISTS tags;
DROP TABLE IF EXISTS config_subscriptions;
DROP TABLE IF EXISTS config_entries;
DROP TABLE IF EXISTS indexer_assignments;
DROP TABLE IF EXISTS indexers;
DROP TABLE IF EXISTS service_capabilities;
DROP TABLE IF EXISTS services;
