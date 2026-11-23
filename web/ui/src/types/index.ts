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

// Per-indexer stats measured from real searches over the last 30 days
// (GET /api/v1/indexers/stats). Rates are 0–1.
export interface IndexerStats {
  indexer_id: string;
  searches: number;
  failures: number;
  success_rate: number;
  median_ms: number;
  p90_ms: number;
  avg_results: number;
  avg_with_seeders: number;
  median_seeders: number;
  behind_cloudflare: number;
  solve_rate: number;
  error_kinds?: Record<string, number>;
  comparisons: number;
  unique_share: number;
  avg_unique: number;
  last_search?: string;
}

export interface IndexerStatsResponse {
  window_days: number;
  indexers: IndexerStats[];
}

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
  health_status: "unknown" | "ok" | "definition_missing" | "failing";
  health_message?: string;
  health_checked_at?: string;
  failure_count?: number;
  last_error?: string;
  last_success_at?: string;
  // Set when last_error matches the Cloudflare IP-ban signature — the
  // UI offers "Rotate VPN IP" in context.
  likely_ip_ban?: boolean;
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
