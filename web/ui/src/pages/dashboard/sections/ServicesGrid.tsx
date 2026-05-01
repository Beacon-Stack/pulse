import { useEffect, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";
import type { ServiceSummary } from "@/api/dashboard";
import { card } from "@/lib/styles";
import { formatBytes } from "@/lib/utils";
import StatusBadge from "@/components/StatusBadge";
import Gauge from "@/components/charts/Gauge";

interface ServicesGridProps {
  services: ServiceSummary[];
}

// Memory gauge reference. When no real cgroup limit is set on the
// container (host RAM is reported as the limit), %-of-host is misleading
// — every service pegs near 0%. Scale to this fixed reference instead so
// 50 MB ≈ 1% and 4 GB ≈ 100%; over-4GB services peg at 100% with the
// real number visible in the label. If a real cgroup limit exists, that
// wins and we use %-of-actual-limit.
const MEM_GAUGE_REFERENCE_BYTES = 4 * 1024 * 1024 * 1024; // 4 GiB

/**
 * Grid of per-service cards. Each card shows live container stats from
 * Docker (or em-dashes when Docker stats are disabled). Click → opens the
 * drill-down drawer via the ?service=<id> query param.
 */
export default function ServicesGrid({ services }: ServicesGridProps) {
  const [, setParams] = useSearchParams();

  if (!services.length) {
    return (
      <div
        style={{
          ...card,
          color: "var(--color-text-muted)",
          fontSize: 13,
          textAlign: "center",
          padding: 40,
        }}
      >
        No services registered yet.
      </div>
    );
  }

  return (
    <div
      style={{
        display: "grid",
        gridTemplateColumns: "repeat(auto-fill, minmax(340px, 1fr))",
        gap: 16,
      }}
    >
      {services.map((svc) => (
        <ServiceCard
          key={svc.id}
          svc={svc}
          onClick={() =>
            setParams(
              (prev) => {
                const next = new URLSearchParams(prev);
                next.set("service", svc.id);
                return next;
              },
              { replace: false },
            )
          }
        />
      ))}
    </div>
  );
}

function ServiceCard({
  svc,
  onClick,
}: {
  svc: ServiceSummary;
  onClick: () => void;
}) {
  const c = svc.container;

  // Per-card peak network rate, so the network gauge has a moving needle
  // even though raw rate is unbounded. Resets when the card unmounts.
  const netPeakRef = useRef(0);
  const [, force] = useState(0);
  useEffect(() => {
    if (!c) return;
    const cur = c.net_rx_rate_bps + c.net_tx_rate_bps;
    if (cur > netPeakRef.current) {
      netPeakRef.current = cur;
      force((n) => n + 1);
    }
  }, [c]);

  const cpuPct = c?.cpu_percent ?? 0;
  const memUsed = c?.mem_usage_bytes ?? 0;
  const memLimit = c?.mem_limit_bytes ?? 0;

  // Memory gauge scaling: prefer real cgroup limit when present, fall
  // back to MEM_GAUGE_REFERENCE_BYTES (4 GiB) so the gauge is visually
  // useful at typical service memory profiles.
  const memScale =
    memLimit > 0 && memLimit < 8 * 1024 * 1024 * 1024 ? memLimit : MEM_GAUGE_REFERENCE_BYTES;
  const memPct = c ? Math.min(100, (memUsed / memScale) * 100) : 0;

  // Network gauge — peak-relative on this card.
  const netCurrent = c ? c.net_rx_rate_bps + c.net_tx_rate_bps : 0;
  const netPct = netPeakRef.current > 0 ? (netCurrent / netPeakRef.current) * 100 : 0;

  return (
    <button
      onClick={onClick}
      style={{
        ...card,
        padding: 16,
        textAlign: "left",
        cursor: "pointer",
        background: "var(--color-bg-surface)",
        border: "1px solid var(--color-border-subtle)",
        transition: "border-color 150ms",
      }}
      onMouseEnter={(e) => (e.currentTarget.style.borderColor = "var(--color-border-strong)")}
      onMouseLeave={(e) => (e.currentTarget.style.borderColor = "var(--color-border-subtle)")}
    >
      <div
        style={{
          display: "flex",
          alignItems: "flex-start",
          justifyContent: "space-between",
          marginBottom: 14,
        }}
      >
        <div>
          <div
            style={{
              fontSize: 14,
              fontWeight: 600,
              color: "var(--color-text-primary)",
            }}
          >
            {svc.name}
          </div>
          <div
            style={{
              fontSize: 11,
              color: "var(--color-text-muted)",
              marginTop: 2,
            }}
          >
            {svc.type}
          </div>
        </div>
        <StatusBadge status={svc.status} />
      </div>

      <div
        style={{
          display: "grid",
          gridTemplateColumns: "repeat(3, 1fr)",
          gap: 8,
          justifyItems: "center",
        }}
      >
        <CardGauge
          title="CPU"
          gauge={
            <Gauge
              value={cpuPct}
              size={64}
              color="var(--color-accent)"
              label={c ? `${cpuPct.toFixed(1)}%` : "—"}
            />
          }
          line=""
        />
        <CardGauge
          title="MEMORY"
          gauge={
            <Gauge
              value={memPct}
              size={64}
              color="var(--color-info)"
              label={c ? formatBytes(memUsed) : "—"}
            />
          }
          line=""
        />
        <CardGauge
          title="NETWORK"
          gauge={
            <Gauge
              value={netPct}
              size={64}
              color="var(--color-success)"
              label={c ? `${formatBytes(netCurrent)}/s` : "—"}
            />
          }
          line=""
        />
      </div>
    </button>
  );
}

function CardGauge({ title, gauge }: { title: string; gauge: React.ReactNode; line: string }) {
  return (
    <div style={{ display: "flex", flexDirection: "column", alignItems: "center", gap: 4, minWidth: 0 }}>
      <div
        style={{
          fontSize: 9,
          color: "var(--color-text-muted)",
          letterSpacing: "0.08em",
          fontWeight: 600,
        }}
      >
        {title}
      </div>
      {gauge}
    </div>
  );
}
