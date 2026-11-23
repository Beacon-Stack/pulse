import type { CatalogSiteHealth } from "@/types";
import { verdictInfo, siteHealthParts, healthTooltip } from "@/lib/catalogHealth";

// A verdict badge ("Works", "Down"...) and one stats line for a catalog
// indexer, from the background tester. Details on hover.

const toneColor = {
  good: "var(--color-success)",
  warn: "var(--color-warning)",
  bad: "var(--color-danger)",
  muted: "var(--color-text-muted)",
};

export function VerdictBadge({ health, untested }: { health?: CatalogSiteHealth; untested?: boolean }) {
  if (!health) {
    if (!untested) return null;
    return (
      <span
        title="Public indexers in your categories are test-searched in the background every few hours."
        style={{ fontSize: 10, fontWeight: 600, padding: "2px 6px", borderRadius: 3, color: "var(--color-text-muted)", background: "var(--color-bg-subtle)", whiteSpace: "nowrap" }}
      >
        Not tested yet
      </span>
    );
  }
  const v = verdictInfo[health.verdict];
  const color = toneColor[v.tone];
  return (
    <span
      title={healthTooltip(health)}
      style={{ fontSize: 10, fontWeight: 600, padding: "2px 6px", borderRadius: 3, color, background: `color-mix(in srgb, ${color} 14%, transparent)`, whiteSpace: "nowrap" }}
    >
      {v.label}
    </span>
  );
}

export function HealthLine({ health }: { health?: CatalogSiteHealth }) {
  if (!health) return null;
  const parts = siteHealthParts(health);
  if (parts.length === 0) return null;
  return (
    <div title={healthTooltip(health)} style={{ fontSize: 11, color: "var(--color-text-muted)", display: "flex", flexWrap: "wrap", columnGap: 4 }}>
      {parts.map((p, i) => (
        <span key={p.text} style={{ whiteSpace: "nowrap", color: p.tone ? toneColor[p.tone] : undefined }}>
          {p.text}
          {i < parts.length - 1 ? " ·" : ""}
        </span>
      ))}
    </div>
  );
}
