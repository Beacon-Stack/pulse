import type { ActiveImport } from "@/api/dashboard";
import { formatBytes } from "@/lib/utils";
import ActivePanel, { type ActiveColumn } from "./ActivePanel";

interface ImportsPanelProps {
  rows: ActiveImport[];
  total: number;
}

const columns: ActiveColumn<ActiveImport>[] = [
  { header: "Title", cell: (r) => r.title || "—", width: 280 },
  { header: "Service", cell: (r) => r.service_name, width: 70 },
  { header: "Status", cell: (r) => <StatusPill status={r.status} />, width: 100 },
  { header: "Progress", cell: (r) => <ProgressCell pct={r.progress * 100} /> },
  { header: "Size", cell: (r) => (r.size > 0 ? formatBytes(r.size) : "—"), numeric: true, width: 80 },
];

export default function ImportsPanel({ rows, total }: ImportsPanelProps) {
  return (
    <ActivePanel
      title="Active imports"
      subtitle="Pilot + Prism queue"
      rows={rows}
      total={total}
      columns={columns}
      rowKey={(r) => `${r.service_id}:${r.title}:${r.grabbed_at}`}
    />
  );
}

const statusColors: Record<string, string> = {
  downloading: "var(--color-info)",
  queued: "var(--color-text-muted)",
  paused: "var(--color-warning)",
  failed: "var(--color-status-offline)",
  completed: "var(--color-success)",
};

function StatusPill({ status }: { status: string }) {
  const color = statusColors[status.toLowerCase()] ?? "var(--color-text-muted)";
  return (
    <span
      style={{
        display: "inline-flex",
        alignItems: "center",
        gap: 4,
        padding: "1px 8px",
        borderRadius: 4,
        fontSize: 10,
        color,
        background: `color-mix(in srgb, ${color} 12%, transparent)`,
      }}
    >
      {status}
    </span>
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
