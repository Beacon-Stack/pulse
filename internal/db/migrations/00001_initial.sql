-- +goose Up

-- ── Services ─────────────────────────────────────────────────────────────────
CREATE TABLE services (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    type        TEXT NOT NULL,
    api_url     TEXT NOT NULL,
    api_key     TEXT NOT NULL DEFAULT '',
    health_url  TEXT NOT NULL DEFAULT '',
    version     TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'unknown',
    last_seen   TEXT NOT NULL DEFAULT '',
    registered  TEXT NOT NULL DEFAULT '',
    metadata    TEXT NOT NULL DEFAULT '{}',
    UNIQUE(name, type)
);

-- ── Service Capabilities ─────────────────────────────────────────────────────
CREATE TABLE service_capabilities (
    id          TEXT PRIMARY KEY,
    service_id  TEXT NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    capability  TEXT NOT NULL,
    UNIQUE(service_id, capability)
);

CREATE INDEX idx_service_capabilities_service ON service_capabilities(service_id);

-- ── Indexers ─────────────────────────────────────────────────────────────────
CREATE TABLE indexers (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    kind        TEXT NOT NULL DEFAULT 'torznab',
    enabled     BOOLEAN NOT NULL DEFAULT TRUE,
    priority    INTEGER NOT NULL DEFAULT 25,
    url         TEXT NOT NULL,
    api_key     TEXT NOT NULL DEFAULT '',
    settings    TEXT NOT NULL DEFAULT '{}',
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

-- ── Indexer Assignments ──────────────────────────────────────────────────────
CREATE TABLE indexer_assignments (
    id          TEXT PRIMARY KEY,
    indexer_id  TEXT NOT NULL REFERENCES indexers(id) ON DELETE CASCADE,
    service_id  TEXT NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    overrides   TEXT NOT NULL DEFAULT '{}',
    UNIQUE(indexer_id, service_id)
);

CREATE INDEX idx_indexer_assignments_service ON indexer_assignments(service_id);
CREATE INDEX idx_indexer_assignments_indexer ON indexer_assignments(indexer_id);

-- ── Shared Configuration ─────────────────────────────────────────────────────
CREATE TABLE config_entries (
    id          TEXT PRIMARY KEY,
    namespace   TEXT NOT NULL,
    key         TEXT NOT NULL,
    value       TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    UNIQUE(namespace, key)
);

CREATE INDEX idx_config_entries_namespace ON config_entries(namespace);

-- ── Config Subscriptions ─────────────────────────────────────────────────────
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

-- ── Filter Presets ───────────────────────────────────────────────────────────
CREATE TABLE filter_presets (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL UNIQUE,
    filters    TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- ── Download Clients ─────────────────────────────────────────────────────────
CREATE TABLE download_clients (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    kind        TEXT NOT NULL,
    protocol    TEXT NOT NULL,
    enabled     BOOLEAN NOT NULL DEFAULT TRUE,
    priority    INTEGER NOT NULL DEFAULT 1,
    host        TEXT NOT NULL,
    port        INTEGER NOT NULL,
    use_ssl     BOOLEAN NOT NULL DEFAULT FALSE,
    username    TEXT NOT NULL DEFAULT '',
    password    TEXT NOT NULL DEFAULT '',
    category    TEXT NOT NULL DEFAULT '',
    directory   TEXT NOT NULL DEFAULT '',
    settings    TEXT NOT NULL DEFAULT '{}',
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS download_clients;
DROP TABLE IF EXISTS filter_presets;
DROP TABLE IF EXISTS indexer_tags;
DROP TABLE IF EXISTS service_tags;
DROP TABLE IF EXISTS tags;
DROP TABLE IF EXISTS config_subscriptions;
DROP TABLE IF EXISTS config_entries;
DROP TABLE IF EXISTS indexer_assignments;
DROP TABLE IF EXISTS indexers;
DROP TABLE IF EXISTS service_capabilities;
DROP TABLE IF EXISTS services;
