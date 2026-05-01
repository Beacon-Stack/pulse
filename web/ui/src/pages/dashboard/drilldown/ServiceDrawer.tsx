import { X, ExternalLink } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import Drawer from "@beacon-shared/Drawer";
import {
  useServiceDetail,
  type ContainerStats,
  type RuntimeStats,
  type EnvEntry,
  type LogEntry,
} from "@/api/dashboard";
import { useService } from "@/api/services";
import { formatBytes } from "@/lib/utils";
import StatusBadge from "@/components/StatusBadge";
import Gauge from "@/components/charts/Gauge";

interface ServiceDrawerProps {
  serviceId: string;
  onClose: () => void;
}

export default function ServiceDrawer({ serviceId, onClose }: ServiceDrawerProps) {
  const { data: detail, isLoading } = useServiceDetail(serviceId);
  // Fall back to the registry record while detail loads (smoother first paint).
  const { data: svc } = useService(serviceId);
  const headerName = detail?.service.name ?? svc?.name ?? "—";
  const headerType = detail?.service.type ?? svc?.type ?? "";
  const headerVer = detail?.service.version ?? svc?.version ?? "";
  const externalURL = svc?.api_url;

  return (
    <Drawer onClose={onClose} width={620}>
      <div
        style={{
          display: "flex",
          alignItems: "center",
          padding: "14px 16px",
          borderBottom: "1px solid var(--color-border-subtle)",
        }}
      >
        <div style={{ flex: 1 }}>
          <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
            <h3
              style={{
                margin: 0,
                fontSize: 16,
                fontWeight: 600,
                color: "var(--color-text-primary)",
              }}
            >
              {headerName}
            </h3>
            {detail?.service.status && <StatusBadge status={detail.service.status} />}
          </div>
          <div
            style={{
              fontSize: 12,
              color: "var(--color-text-muted)",
              marginTop: 2,
              fontFamily: "var(--font-family-mono)",
            }}
          >
            {headerType}
            {headerVer && <span> · {headerVer}</span>}
          </div>
        </div>
        {externalURL && (
          <a
            href={externalURL}
            target="_blank"
            rel="noopener noreferrer"
            style={{
              padding: 6,
              color: "var(--color-text-secondary)",
              borderRadius: 6,
              display: "inline-flex",
              alignItems: "center",
            }}
            title="Open service UI"
          >
            <ExternalLink size={14} />
          </a>
        )}
        <button
          onClick={onClose}
          style={{
            padding: 6,
            background: "transparent",
            color: "var(--color-text-secondary)",
            border: "none",
            cursor: "pointer",
            borderRadius: 6,
          }}
          title="Close (Esc)"
        >
          <X size={16} />
        </button>
      </div>

      <div style={{ flex: 1, overflowY: "auto", padding: 16 }}>
        {isLoading && !detail ? (
          <div style={{ color: "var(--color-text-muted)", fontSize: 13 }}>Loading…</div>
        ) : (
          <>
            <GaugesRow container={detail?.container ?? null} runtime={detail?.runtime ?? null} />
            <HostSection runtime={detail?.runtime ?? null} />
            <SpecificsPanel name={headerName} type={headerType} specifics={detail?.specifics} />
            <EnvSection env={detail?.env ?? null} />
            <LogsSection logs={detail?.logs ?? null} />
          </>
        )}
      </div>
    </Drawer>
  );
}

// ── 3 gauges row ────────────────────────────────────────────────────────────

function GaugesRow({
  container,
  runtime,
}: {
  container: ContainerStats | null;
  runtime: RuntimeStats | null;
}) {
  // Track per-drawer-instance peak network rate. Resets on drawer close.
  const netPeakRef = useRef(0);
  const [, force] = useState(0);
  useEffect(() => {
    if (!container) return;
    const netCur = container.net_rx_rate_bps + container.net_tx_rate_bps;
    if (netCur > netPeakRef.current) {
      netPeakRef.current = netCur;
      force((n) => n + 1);
    }
  }, [container]);

  const cpu = container?.cpu_percent ?? 0;
  const memUsed = container?.mem_usage_bytes ?? 0;
  const memLimit = container?.mem_limit_bytes ?? 0;
  // Memory gauge scale: real cgroup limit when set, otherwise a 4 GiB
  // reference so a typical service memory profile (50 MB – 2 GB) looks
  // visually meaningful. Without this, the gauge pegs at ~0% on machines
  // with no per-container memory limits, since Docker reports host RAM
  // as the limit. Over-4 GB usage clamps the gauge at 100%.
  const MEM_GAUGE_REFERENCE = 4 * 1024 * 1024 * 1024; // 4 GiB
  const limitIsReal = memLimit > 0 && memLimit < 8 * 1024 * 1024 * 1024;
  const memScale = limitIsReal ? memLimit : MEM_GAUGE_REFERENCE;
  const memPct = container ? Math.min(100, (memUsed / memScale) * 100) : 0;
  const memSecondary = limitIsReal ? `of ${formatBytes(memLimit)}` : `of ${formatBytes(memScale)} ref`;

  const netCurrent = container ? container.net_rx_rate_bps + container.net_tx_rate_bps : 0;
  const netPct = netPeakRef.current > 0 ? (netCurrent / netPeakRef.current) * 100 : 0;

  const noContainer = !container;

  return (
    <Section title="Resources">
      <div
        style={{
          display: "grid",
          gridTemplateColumns: "repeat(3, 1fr)",
          gap: 12,
          alignItems: "start",
          justifyItems: "center",
          padding: "8px 0",
        }}
      >
        <GaugeBlock
          title="CPU"
          gauge={
            <Gauge
              value={cpu}
              size={96}
              color="var(--color-accent)"
              label={noContainer ? "—" : `${cpu.toFixed(1)}%`}
            />
          }
          primary={runtime ? `${runtime.num_cpu} core${runtime.num_cpu === 1 ? "" : "s"}` : "—"}
          secondary=""
        />
        <GaugeBlock
          title="Memory"
          gauge={
            <Gauge
              value={memPct}
              size={96}
              color="var(--color-info)"
              label={noContainer ? "—" : `${memPct.toFixed(0)}%`}
            />
          }
          primary={noContainer ? "—" : formatBytes(memUsed)}
          secondary={memSecondary}
        />
        <GaugeBlock
          title="Network"
          gauge={
            <Gauge
              value={netPct}
              size={96}
              color="var(--color-success)"
              label={noContainer ? "—" : `${netPct.toFixed(0)}%`}
            />
          }
          primary={noContainer ? "—" : `${formatBytes(netCurrent)}/s`}
          secondary={netPeakRef.current > 0 ? `peak ${formatBytes(netPeakRef.current)}/s` : ""}
        />
      </div>
      {noContainer && (
        <div style={{ fontSize: 11, color: "var(--color-text-muted)", textAlign: "center", marginTop: 6 }}>
          Container stats not available — set PULSE_DASHBOARD_DOCKER_SOCKET to enable.
        </div>
      )}
    </Section>
  );
}

// GaugeBlock renders a gauge with its title above and primary/secondary
// values below. Keeping the values OUTSIDE the ring means long strings
// like "5.2 MB/s" or "peak 12 MB/s" don't have to fit inside it.
function GaugeBlock({
  title,
  gauge,
  primary,
  secondary,
}: {
  title: string;
  gauge: React.ReactNode;
  primary: string;
  secondary: string;
}) {
  return (
    <div style={{ display: "flex", flexDirection: "column", alignItems: "center", gap: 4, minWidth: 0 }}>
      <div
        style={{
          fontSize: 11,
          color: "var(--color-text-muted)",
          textTransform: "uppercase",
          letterSpacing: "0.06em",
          marginBottom: 4,
        }}
      >
        {title}
      </div>
      {gauge}
      <div
        style={{
          fontSize: 13,
          fontWeight: 500,
          color: "var(--color-text-primary)",
          fontVariantNumeric: "tabular-nums",
          marginTop: 6,
          textAlign: "center",
        }}
      >
        {primary}
      </div>
      {secondary && (
        <div
          style={{
            fontSize: 11,
            color: "var(--color-text-muted)",
            fontVariantNumeric: "tabular-nums",
            textAlign: "center",
          }}
        >
          {secondary}
        </div>
      )}
    </div>
  );
}

// ── Host info ───────────────────────────────────────────────────────────────

function HostSection({ runtime }: { runtime: RuntimeStats | null }) {
  if (!runtime) {
    return (
      <Section title="Host">
        <Empty text="Runtime endpoint did not respond." />
      </Section>
    );
  }
  return (
    <Section title="Host">
      <Stat label="Hostname" value={runtime.hostname || "—"} mono />
      <Stat label="OS" value={`${runtime.goos}/${runtime.goarch}`} mono />
      <Stat label="CPUs" value={runtime.num_cpu.toString()} mono />
      <Stat label="Uptime" value={formatUptime(runtime.uptime_seconds)} mono />
      <Stat label="Go version" value={runtime.go_version} mono />
      <Stat label="Goroutines" value={runtime.goroutines.toString()} mono />
      <Stat label="Heap in use" value={formatBytes(runtime.heap_in_use_bytes)} mono />
      <Stat label="GC cycles" value={runtime.num_gc.toString()} mono />
      <Stat label="Last GC pause" value={`${(runtime.last_gc_pause_ns / 1000).toFixed(0)} µs`} mono />
    </Section>
  );
}

function formatUptime(seconds: number): string {
  if (seconds < 60) return `${seconds}s`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m`;
  const hours = Math.floor(seconds / 3600);
  if (hours < 24) return `${hours}h ${Math.floor((seconds % 3600) / 60)}m`;
  const days = Math.floor(hours / 24);
  return `${days}d ${hours % 24}h`;
}

// ── Env ─────────────────────────────────────────────────────────────────────

function EnvSection({ env }: { env: EnvEntry[] | null }) {
  if (!env || env.length === 0) {
    return (
      <Section title="Environment">
        <Empty text="No env data." />
      </Section>
    );
  }
  return (
    <Section title="Environment">
      <div style={{ display: "flex", flexDirection: "column" }}>
        {env.map((e) => (
          <div
            key={e.key}
            style={{
              display: "flex",
              gap: 8,
              padding: "4px 0",
              fontSize: 12,
              borderTop: "1px solid var(--color-border-subtle)",
              alignItems: "baseline",
            }}
          >
            <span
              style={{
                color: "var(--color-text-secondary)",
                fontFamily: "var(--font-family-mono)",
                whiteSpace: "nowrap",
              }}
            >
              {e.key}
            </span>
            <span
              style={{
                marginLeft: "auto",
                color: e.redacted ? "var(--color-text-muted)" : "var(--color-text-primary)",
                fontFamily: "var(--font-family-mono)",
                fontStyle: e.redacted ? "italic" : "normal",
                wordBreak: "break-all",
                textAlign: "right",
              }}
            >
              {e.value}
            </span>
          </div>
        ))}
      </div>
    </Section>
  );
}

// ── Logs ────────────────────────────────────────────────────────────────────

const logLevelColor: Record<string, string> = {
  DEBUG: "var(--color-text-muted)",
  INFO: "var(--color-text-secondary)",
  WARN: "var(--color-warning)",
  ERROR: "var(--color-status-offline)",
};

function LogsSection({ logs }: { logs: LogEntry[] | null }) {
  if (!logs || logs.length === 0) {
    return (
      <Section title="Logs">
        <Empty text="Service did not return logs (no /api/v1/system/logs endpoint, or buffer empty)." />
      </Section>
    );
  }
  return (
    <Section title={`Logs · last ${logs.length}`}>
      <div
        style={{
          background: "var(--color-bg-canvas)",
          border: "1px solid var(--color-border-subtle)",
          borderRadius: 6,
          padding: 8,
          maxHeight: 240,
          overflowY: "auto",
          fontFamily: "var(--font-family-mono)",
          fontSize: 11,
          lineHeight: 1.5,
        }}
      >
        {/* Reverse so newest is on top — easier to scan. */}
        {[...logs].reverse().map((e, i) => {
          const fieldText = formatLogFields(e.fields);
          const full = fieldText ? `${e.message} ${fieldText}` : e.message;
          return (
            <div key={i} style={{ display: "flex", gap: 8, padding: "1px 0", whiteSpace: "nowrap" }}>
              <span style={{ color: "var(--color-text-muted)", flexShrink: 0 }}>
                {formatLogTime(e.time)}
              </span>
              <span
                style={{
                  color: logLevelColor[e.level] ?? "var(--color-text-secondary)",
                  flexShrink: 0,
                  width: 40,
                }}
              >
                {e.level}
              </span>
              <span
                style={{
                  color: "var(--color-text-primary)",
                  overflow: "hidden",
                  textOverflow: "ellipsis",
                }}
                title={full}
              >
                {e.message}
                {fieldText && (
                  <span style={{ color: "var(--color-text-muted)", marginLeft: 6 }}>{fieldText}</span>
                )}
              </span>
            </div>
          );
        })}
      </div>
    </Section>
  );
}

// formatLogFields renders structured log attributes (method, path, etc.)
// as compact "key=value" pairs, JSON-stringified when nested. Without this
// every HTTP log line in Haul's buffer just says "request" and the useful
// info — method/path/duration_ms — is invisible.
function formatLogFields(fields: Record<string, unknown> | null | undefined): string {
  if (!fields) return "";
  const parts: string[] = [];
  for (const [k, v] of Object.entries(fields)) {
    if (v === null || v === undefined || v === "") continue;
    const s = typeof v === "object" ? JSON.stringify(v) : String(v);
    parts.push(`${k}=${s}`);
  }
  return parts.join(" ");
}

function formatLogTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "—";
  const h = String(d.getHours()).padStart(2, "0");
  const m = String(d.getMinutes()).padStart(2, "0");
  const s = String(d.getSeconds()).padStart(2, "0");
  return `${h}:${m}:${s}`;
}

// ── Service-specific ────────────────────────────────────────────────────────

interface HaulSpecifics {
  stats?: {
    download_speed: number;
    upload_speed: number;
    active_downloads: number;
    peers_connected: number;
  };
  torrents?: Array<{
    id?: string;
    name?: string;
    progress?: number;
    download_speed?: number;
    peers?: number;
    state?: string;
  }>;
}

interface QueueSpecifics {
  queue?: {
    items?: Array<{ id?: string; title?: string; status?: string; progress?: number; size?: number }>;
    total?: number;
    records?: Array<{ title?: string; status?: string; progress?: number; size?: number }>;
  };
}

function SpecificsPanel({
  name,
  type,
  specifics,
}: {
  name: string;
  type: string;
  specifics: Record<string, unknown> | null | undefined;
}) {
  if (!specifics) return null;

  const isHaul = name === "Haul" || type === "download-client";
  const isQueueOwner = name === "Pilot" || name === "Prism" || type === "media-manager";

  if (isHaul) return <HaulSection s={specifics as unknown as HaulSpecifics} />;
  if (isQueueOwner) return <QueueSection s={specifics as unknown as QueueSpecifics} />;
  return null;
}

function HaulSection({ s }: { s: HaulSpecifics }) {
  return (
    <Section title="Active downloads">
      {s.stats && (
        <div style={{ display: "flex", gap: 16, fontSize: 12, marginBottom: 12, flexWrap: "wrap" }}>
          <Pill label="↓" value={`${formatBytes(s.stats.download_speed)}/s`} />
          <Pill label="↑" value={`${formatBytes(s.stats.upload_speed)}/s`} />
          <Pill label="active" value={s.stats.active_downloads.toString()} />
          <Pill label="peers" value={s.stats.peers_connected.toString()} />
        </div>
      )}
      {s.torrents?.length ? (
        <Table
          headers={["Name", "Progress", "Speed", "Peers"]}
          rows={s.torrents.map((t) => [
            t.name ?? "—",
            <ProgressCell pct={(t.progress ?? 0) * 100} key="p" />,
            t.download_speed ? `${formatBytes(t.download_speed)}/s` : "—",
            (t.peers ?? 0).toString(),
          ])}
        />
      ) : (
        <Empty text="Nothing downloading right now." />
      )}
    </Section>
  );
}

function QueueSection({ s }: { s: QueueSpecifics }) {
  // Pilot/Prism's /api/v1/queue can return either {items: [...]} or
  // {records: [...]} depending on which version of the contract; tolerate both.
  const items = s.queue?.records ?? s.queue?.items ?? [];
  return (
    <Section title="Queue">
      {items.length ? (
        <Table
          headers={["Title", "Status", "Progress", "Size"]}
          rows={items.slice(0, 20).map((it) => [
            it.title ?? "—",
            it.status ?? "—",
            <ProgressCell pct={(it.progress ?? 0)} key="p" />,
            it.size ? formatBytes(it.size) : "—",
          ])}
        />
      ) : (
        <Empty text="Queue is empty." />
      )}
    </Section>
  );
}

// ── Shared bits ─────────────────────────────────────────────────────────────

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section style={{ marginBottom: 24 }}>
      <h4
        style={{
          margin: "0 0 8px",
          fontSize: 11,
          fontWeight: 600,
          color: "var(--color-text-muted)",
          textTransform: "uppercase",
          letterSpacing: "0.06em",
        }}
      >
        {title}
      </h4>
      {children}
    </section>
  );
}

function Stat({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div
      style={{
        display: "flex",
        justifyContent: "space-between",
        fontSize: 12,
        padding: "4px 0",
      }}
    >
      <span style={{ color: "var(--color-text-secondary)" }}>{label}</span>
      <span
        style={{
          color: "var(--color-text-primary)",
          fontFamily: mono ? "var(--font-family-mono)" : undefined,
        }}
      >
        {value}
      </span>
    </div>
  );
}

function Empty({ text }: { text: string }) {
  return (
    <div
      style={{
        fontSize: 12,
        color: "var(--color-text-muted)",
        padding: "8px 0",
      }}
    >
      {text}
    </div>
  );
}

function Pill({ label, value }: { label: string; value: string }) {
  return (
    <span
      style={{
        display: "inline-flex",
        alignItems: "center",
        gap: 4,
        padding: "2px 8px",
        borderRadius: 4,
        fontSize: 11,
        background: "var(--color-bg-canvas)",
        border: "1px solid var(--color-border-subtle)",
        fontFamily: "var(--font-family-mono)",
      }}
    >
      <span style={{ color: "var(--color-text-muted)" }}>{label}</span>
      <span style={{ color: "var(--color-text-primary)" }}>{value}</span>
    </span>
  );
}

function Table({
  headers,
  rows,
}: {
  headers: string[];
  rows: React.ReactNode[][];
}) {
  return (
    <div style={{ overflowX: "auto" }}>
      <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 12 }}>
        <thead>
          <tr style={{ borderBottom: "1px solid var(--color-border-subtle)" }}>
            {headers.map((h) => (
              <th
                key={h}
                style={{
                  textAlign: "left",
                  padding: "6px 8px",
                  fontSize: 10,
                  fontWeight: 600,
                  color: "var(--color-text-muted)",
                  textTransform: "uppercase",
                  letterSpacing: "0.06em",
                }}
              >
                {h}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((r, i) => (
            <tr
              key={i}
              style={{
                borderBottom: "1px solid var(--color-border-subtle)",
              }}
            >
              {r.map((c, j) => (
                <td
                  key={j}
                  style={{
                    padding: "6px 8px",
                    color: "var(--color-text-primary)",
                    maxWidth: 240,
                    overflow: "hidden",
                    textOverflow: "ellipsis",
                    whiteSpace: "nowrap",
                  }}
                >
                  {c}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
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
      <span style={{ fontFamily: "var(--font-family-mono)", color: "var(--color-text-muted)", fontSize: 11 }}>
        {clamped.toFixed(0)}%
      </span>
    </div>
  );
}
