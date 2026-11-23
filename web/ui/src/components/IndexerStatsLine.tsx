import type { IndexerStats } from "@/types";
import { statsParts, statsTooltip } from "@/lib/indexerStats";

// One line of stats measured from real searches, with details on hover.

const toneColor = {
  good: "var(--color-success)",
  warn: "var(--color-warning)",
  bad: "var(--color-danger)",
};

export default function IndexerStatsLine({ stats, windowDays }: { stats?: IndexerStats; windowDays: number }) {
  if (!stats || stats.searches === 0) {
    return (
      <div style={{ fontSize: 11, color: "var(--color-text-muted)", marginTop: 4 }}>
        No searches yet
      </div>
    );
  }
  const parts = statsParts(stats);
  return (
    <div
      title={statsTooltip(stats, windowDays)}
      style={{ fontSize: 11, color: "var(--color-text-muted)", marginTop: 4, display: "flex", flexWrap: "wrap", columnGap: 4 }}
    >
      {parts.map((p, i) => (
        <span key={p.text} style={{ whiteSpace: "nowrap", color: p.tone ? toneColor[p.tone] : undefined }}>
          {p.text}
          {i < parts.length - 1 ? " ·" : ""}
        </span>
      ))}
    </div>
  );
}
