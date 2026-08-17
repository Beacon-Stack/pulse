import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { apiFetch } from "./client";

// Backend lives at pulse/internal/core/dashboard/aggregator.go — keep
// these types in sync with that package's response shapes.

export interface ContainerStats {
  cpu_percent: number;
  mem_usage_bytes: number;
  mem_limit_bytes: number;
  net_rx_bytes: number;
  net_tx_bytes: number;
  net_rx_rate_bps: number;
  net_tx_rate_bps: number;
  block_read_bytes: number;
  block_write_bytes: number;
  health_status: string;
}

export interface ServiceSummary {
  id: string;
  name: string;
  type: string;
  status: string;
  version: string;
  container?: ContainerStats | null;
}

export interface HaulSummary {
  download_speed: number;
  upload_speed: number;
  active_downloads: number;
  active_uploads: number;
  peers_connected: number;
}

export interface VPNSummary {
  reachable: boolean;
  connected: boolean;
  public_ip: string;
  country: string;
  port_forwarded: number;
  provider: string;
  dns_status: string;
}

export interface ActiveDownload {
  service_id: string;
  service_name: string;
  name: string;
  progress: number;
  download_rate: number;
  upload_rate: number;
  peers: number;
  eta_seconds: number;
  status: string;
}

export interface ActiveImport {
  service_id: string;
  service_name: string;
  title: string;
  status: string;
  size: number;
  downloaded_bytes: number;
  progress: number;
  grabbed_at: string;
}

export interface DashboardOverview {
  services: ServiceSummary[];
  haul?: HaulSummary | null;
  vpn?: VPNSummary | null;
  active_downloads?: ActiveDownload[] | null;
  active_downloads_total?: number;
  active_imports?: ActiveImport[] | null;
  active_imports_total?: number;
}

export interface RuntimeStats {
  goroutines: number;
  heap_alloc_bytes: number;
  heap_in_use_bytes: number;
  heap_objects: number;
  num_gc: number;
  last_gc_pause_ns: number;
  uptime_seconds: number;
  go_version: string;
  goos: string;
  goarch: string;
  num_cpu: number;
  hostname: string;
}

export interface EnvEntry {
  key: string;
  value: string;
  redacted: boolean;
}

export interface LogEntry {
  time: string;
  level: string;
  message: string;
  fields?: Record<string, unknown> | null;
}

export interface ServiceDetail {
  service: ServiceSummary;
  container?: ContainerStats | null;
  runtime?: RuntimeStats | null;
  env?: EnvEntry[] | null;
  logs?: LogEntry[] | null;
  specifics?: Record<string, unknown> | null;
}

/** Polled every 2s — keep the payload small. */
export function useDashboardOverview() {
  return useQuery({
    queryKey: ["dashboard", "overview"],
    queryFn: () => apiFetch<DashboardOverview>("/dashboard/overview"),
    refetchInterval: 2000,
    refetchOnWindowFocus: false,
  });
}

export interface RotateVPNResponse {
  old_ip: string;
  new_ip: string;
  attempts: number;
  changed: boolean;
  message?: string;
}

/**
 * Cycles the VPN tunnel until the provider assigns a new exit IP (up to
 * 3 attempts, ~30s-2min). Clears Cloudflare sessions server-side. The
 * backend rate-limits to one rotation per 5 minutes.
 */
export function useRotateVPN() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => apiFetch<RotateVPNResponse>("/vpn/rotate", { method: "POST" }),
    onSuccess: (res) => {
      qc.invalidateQueries({ queryKey: ["dashboard", "overview"] });
      qc.invalidateQueries({ queryKey: ["indexers"] });
      if (res.changed) {
        toast.success(`VPN IP rotated: ${res.old_ip} → ${res.new_ip}`);
      } else {
        toast.warning(res.message ?? "VPN reconnected but the IP did not change");
      }
    },
    onError: (err) => toast.error((err as Error).message),
  });
}

/** Only polls when a service is selected (drawer open). */
export function useServiceDetail(serviceId: string | null) {
  return useQuery({
    queryKey: ["dashboard", "service", serviceId],
    queryFn: () => apiFetch<ServiceDetail>(`/dashboard/services/${serviceId}`),
    refetchInterval: 2000,
    refetchOnWindowFocus: false,
    enabled: !!serviceId,
  });
}
