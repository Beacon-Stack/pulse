import type { ActiveDownload } from "@/api/dashboard";
import { formatBytes } from "@/lib/utils";
import ActivePanel, { type ActiveColumn } from "./ActivePanel";

interface DownloadsPanelProps {
  rows: ActiveDownload[];
  total: number;
}

const columns: ActiveColumn<ActiveDownload>[] = [
  { header: "Name", cell: (r) => r.name || "—", width: 280 },
  { header: "Progress", cell: (r) => <ProgressCell pct={r.progress * 100} /> },
  { header: "↓", cell: (r) => `${formatBytes(r.download_rate)}/s`, numeric: true },
  { header: "↑", cell: (r) => `${formatBytes(r.upload_rate)}/s`, numeric: true },
  { header: "Peers", cell: (r) => r.peers.toString(), numeric: true, width: 56 },
  { header: "ETA", cell: (r) => formatETA(r.eta_seconds), numeric: true, width: 72 },
];

export default function DownloadsPanel({ rows, total }: DownloadsPanelProps) {
  return (
    <ActivePanel
      title="Active downloads"
      subtitle="Live from Haul"
      rows={rows}
      total={total}
      columns={columns}
      rowKey={(r) => `${r.service_id}:${r.name}`}
    />
  );
}

function ProgressCell({ pct }: { pct: number }) {
  const clamped = Math.max(0, Math.min(100, pct));
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
      <div
        style={{
          width: 60,
          height: 4,
          background: "var(--color-border-subtle)",
          borderRadius: 4,
          overflow: "hidden",
        }}
      >
        <div
          style={{
            width: `${clamped}%`,
            height: "100%",
            background: "var(--color-accent)",
          }}
        />
      </div>
      <span
        style={{
          fontVariantNumeric: "tabular-nums",
          color: "var(--color-text-muted)",
          fontSize: 11,
        }}
      >
        {clamped.toFixed(0)}%
      </span>
    </div>
  );
}

function formatETA(seconds: number): string {
  if (seconds <= 0 || !Number.isFinite(seconds)) return "—";
  if (seconds < 60) return `${seconds}s`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h`;
  return `${Math.floor(seconds / 86400)}d`;
}
