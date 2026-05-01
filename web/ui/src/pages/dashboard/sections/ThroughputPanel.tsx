import { ArrowDown, ArrowUp, Users } from "lucide-react";
import { useEffect, useRef } from "react";
import type { HaulSummary } from "@/api/dashboard";
import { card } from "@/lib/styles";
import { formatBytes } from "@/lib/utils";
import Sparkline from "@/components/charts/Sparkline";

interface ThroughputPanelProps {
  haul: HaulSummary;
}

const BUFFER_SIZE = 10;

/**
 * Live throughput card backed by Haul's /api/v1/stats. Rolling buffer of
 * 10 samples × 2s poll = 20s of recent history in the sparkline. Buffer
 * is component-state only — refreshing the page resets it.
 */
export default function ThroughputPanel({ haul }: ThroughputPanelProps) {
  const downBuf = useRef<number[]>([]);
  const upBuf = useRef<number[]>([]);

  useEffect(() => {
    downBuf.current = [...downBuf.current, haul.download_speed].slice(-BUFFER_SIZE);
    upBuf.current = [...upBuf.current, haul.upload_speed].slice(-BUFFER_SIZE);
    // refresh on every poll — refs mutate above and we just need a re-render.
  }, [haul.download_speed, haul.upload_speed]);

  return (
    <div style={{ ...card, padding: 16 }}>
      <div style={{ marginBottom: 12 }}>
        <div
          style={{
            fontSize: 13,
            fontWeight: 600,
            color: "var(--color-text-primary)",
          }}
        >
          Download throughput
        </div>
        <div style={{ fontSize: 11, color: "var(--color-text-muted)", marginTop: 2 }}>
          Live from Haul
        </div>
      </div>

      <div
        style={{
          display: "grid",
          gridTemplateColumns: "1fr 1fr",
          gap: 12,
          marginBottom: 12,
        }}
      >
        <Direction
          icon={<ArrowDown size={14} />}
          color="var(--color-success)"
          label="Down"
          rate={haul.download_speed}
          buffer={downBuf.current}
        />
        <Direction
          icon={<ArrowUp size={14} />}
          color="var(--color-info)"
          label="Up"
          rate={haul.upload_speed}
          buffer={upBuf.current}
        />
      </div>

      <div
        style={{
          display: "flex",
          gap: 16,
          fontSize: 12,
          color: "var(--color-text-secondary)",
          paddingTop: 8,
          borderTop: "1px solid var(--color-border-subtle)",
        }}
      >
        <span>{haul.active_downloads} downloading</span>
        <span>{haul.active_uploads} seeding</span>
        <span style={{ display: "flex", alignItems: "center", gap: 4, marginLeft: "auto" }}>
          <Users size={12} /> {haul.peers_connected}
        </span>
      </div>
    </div>
  );
}

function Direction({
  icon,
  color,
  label,
  rate,
  buffer,
}: {
  icon: React.ReactNode;
  color: string;
  label: string;
  rate: number;
  buffer: number[];
}) {
  return (
    <div>
      <div
        style={{
          display: "flex",
          alignItems: "center",
          gap: 6,
          fontSize: 11,
          textTransform: "uppercase",
          letterSpacing: "0.06em",
          color: "var(--color-text-muted)",
          marginBottom: 4,
        }}
      >
        <span style={{ color }}>{icon}</span>
        {label}
      </div>
      <div
        style={{
          fontSize: 18,
          fontWeight: 600,
          color: "var(--color-text-primary)",
          fontFamily: "var(--font-family-mono)",
        }}
      >
        {formatBytes(rate)}/s
      </div>
      <div style={{ marginTop: 4 }}>
        <Sparkline data={buffer} color={color} height={20} />
      </div>
    </div>
  );
}
