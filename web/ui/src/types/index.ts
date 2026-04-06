// ── Services ─────────────────────────────────────────────────────────────────

export interface Service {
  id: string;
  name: string;
  type: string;
  api_url: string;
  health_url: string;
  version: string;
  status: string;
  last_seen: string;
  registered: string;
  capabilities: string[];
  metadata: string;
}

export interface RegisterServiceInput {
  name: string;
  type: string;
  api_url: string;
  api_key?: string;
  health_url?: string;
  version?: string;
  capabilities?: string[];
  metadata?: string;
}

// ── Config ───────────────────────────────────────────────────────────────────

export interface ConfigEntry {
  namespace: string;
  key: string;
  value: string;
  updated_at: string;
}

export interface SetConfigInput {
  namespace: string;
  key: string;
  value: string;
}

// ── Indexers ─────────────────────────────────────────────────────────────────

export interface Indexer {
  id: string;
  name: string;
  kind: string;
  enabled: boolean;
  priority: number;
  url: string;
  settings: string;
  created_at: string;
  updated_at: string;
}

export interface CreateIndexerInput {
  name: string;
  kind?: string;
  enabled?: boolean;
  priority?: number;
  url: string;
  api_key?: string;
  settings?: string;
}

export interface IndexerAssignment {
  id: string;
  indexerId: string;
  serviceId: string;
  overrides: string;
}

// ── Catalog ──────────────────────────────────────────────────────────────────

export interface CatalogFieldOption {
  name: string;
  value: string;
}

export interface CatalogField {
  name: string;
  type: string; // text, password, checkbox, select, info
  label: string;
  help_text?: string;
  required: boolean;
  default?: string;
  placeholder?: string;
  options?: CatalogFieldOption[];
}

export interface CatalogEntry {
  id: string;
  name: string;
  description: string;
  language: string;
  protocol: string;   // torrent, usenet
  privacy: string;     // public, semi-private, private
  categories: string[];
  urls: string[];
  settings: CatalogField[];
}

export interface CatalogResponse {
  entries: CatalogEntry[];
  total: number;
}

// ── System ───────────────────────────────────────────────────────────────────

export interface SystemStatus {
  status: string;
  version: string;
  uptime: string;
}
