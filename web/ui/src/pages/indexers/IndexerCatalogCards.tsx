import { ChevronRight, Download, Newspaper, Check } from "lucide-react";
import { VerdictBadge, HealthLine, OverlayBadge } from "@/components/CatalogHealthBadge";
import { langDisplay } from "@/lib/languages";
import { needsOverlayNetwork } from "@/lib/catalogHealth";
import IndexerStatsLine from "@/components/IndexerStatsLine";
import type { CatalogEntry, CatalogSiteHealth, IndexerStats } from "@/types";

export const categoryColors: Record<string, string> = {
  Movies: "#3b9eff",
  TV: "#34d399",
  Audio: "#f59e0b",
  Books: "#a78bfa",
  XXX: "#f87171",
  Other: "#6b7280",
};

// Styles

interface CardProps {
  entry: CatalogEntry;
  isAdded: boolean;
  // Faded look for added indexers mixed into the catalog (not in "Added" view).
  dim: boolean;
  // Real-search stats for an added indexer.
  realStats?: IndexerStats;
  health?: CatalogSiteHealth;
  untested: boolean;
  onSelect: () => void;
}

export function GridCard({ entry, isAdded, dim, realStats, health, untested, onSelect }: CardProps) {
  return (
    <button
      onClick={onSelect}
      style={{
        display: "flex",
        flexDirection: "column",
        padding: 16,
        borderRadius: 8,
        border: "1px solid var(--color-border-subtle)",
        background: dim ? "var(--color-bg-base)" : "var(--color-bg-surface)",
        cursor: "pointer",
        textAlign: "left",
        transition: "border-color 120ms ease, box-shadow 120ms ease",
        opacity: dim ? 0.6 : 1,
        position: "relative",
        minHeight: 140,
      }}
      onMouseEnter={(e) => {
        if (!isAdded) {
          e.currentTarget.style.borderColor = "var(--color-accent)";
          e.currentTarget.style.boxShadow = "0 0 0 1px var(--color-accent)";
        }
      }}
      onMouseLeave={(e) => {
        e.currentTarget.style.borderColor = "var(--color-border-subtle)";
        e.currentTarget.style.boxShadow = "none";
      }}
    >
      {/* Top: name + badges */}
      <div style={{ display: "flex", alignItems: "center", gap: 6, marginBottom: 6 }}>
        <span style={{ fontSize: 14, fontWeight: 600, color: "var(--color-text-primary)", flex: 1, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
          {entry.name}
        </span>
        {isAdded ? (
          <span style={{ display: "inline-flex", alignItems: "center", gap: 3, fontSize: 10, fontWeight: 600, color: "var(--color-success)", background: "color-mix(in srgb, var(--color-success) 12%, transparent)", padding: "2px 6px", borderRadius: 3 }}>
            <Check size={10} /> Added
          </span>
        ) : needsOverlayNetwork(entry.urls) ? (
          <OverlayBadge />
        ) : (
          <VerdictBadge health={health} untested={untested} />
        )}
      </div>

      {/* Protocol + privacy badges */}
      <div style={{ display: "flex", gap: 4, marginBottom: 8 }}>
        <ProtocolBadge protocol={entry.protocol} />
        <PrivacyBadge privacy={entry.privacy} />
        {entry.language !== "en-US" && (
          <span style={{ fontSize: 10, padding: "2px 6px", borderRadius: 3, background: "var(--color-bg-subtle)", color: "var(--color-text-muted)" }} title={langDisplay(entry.language).label}>
            {langDisplay(entry.language).flag} {langDisplay(entry.language).label}
          </span>
        )}
      </div>

      {/* Description */}
      <p style={{
        margin: 0,
        fontSize: 12,
        color: "var(--color-text-muted)",
        lineHeight: 1.4,
        flex: 1,
        display: "-webkit-box",
        WebkitLineClamp: 2,
        WebkitBoxOrient: "vertical",
        overflow: "hidden",
      }}>
        {entry.description}
      </p>

      {!isAdded && health && !needsOverlayNetwork(entry.urls) && (
        <div style={{ marginTop: 8 }}>
          <HealthLine health={health} />
        </div>
      )}
      {isAdded && !dim && <IndexerStatsLine stats={realStats} windowDays={30} />}

      {/* Categories */}
      <div style={{ display: "flex", flexWrap: "wrap", gap: 3, marginTop: 10 }}>
        {entry.categories.map((cat) => (
          <span
            key={cat}
            style={{
              fontSize: 10,
              fontWeight: 500,
              padding: "1px 6px",
              borderRadius: 3,
              color: categoryColors[cat] ?? "#6b7280",
              background: `color-mix(in srgb, ${categoryColors[cat] ?? "#6b7280"} 10%, transparent)`,
            }}
          >
            {cat}
          </span>
        ))}
      </div>
    </button>
  );
}

// List Row

export function ListRow({ entry, isAdded, dim, health, untested, onSelect }: CardProps) {
  return (
    <button
      onClick={onSelect}
      style={{
        display: "flex",
        alignItems: "center",
        gap: 12,
        padding: "10px 16px",
        borderRadius: 6,
        border: "1px solid transparent",
        background: dim ? "var(--color-bg-base)" : "transparent",
        cursor: "pointer",
        textAlign: "left",
        transition: "background 100ms ease",
        opacity: dim ? 0.6 : 1,
        width: "100%",
      }}
      onMouseEnter={(e) => { e.currentTarget.style.background = "var(--color-bg-surface)"; }}
      onMouseLeave={(e) => { e.currentTarget.style.background = dim ? "var(--color-bg-base)" : "transparent"; }}
    >
      <span style={{ fontSize: 14, fontWeight: 500, color: "var(--color-text-primary)", width: 180, flexShrink: 0, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
        {entry.name}
      </span>
      <ProtocolBadge protocol={entry.protocol} />
      <PrivacyBadge privacy={entry.privacy} />
      {entry.categories.map((cat) => (
        <span key={cat} style={{ fontSize: 10, fontWeight: 500, padding: "1px 6px", borderRadius: 3, color: categoryColors[cat] ?? "#6b7280", background: `color-mix(in srgb, ${categoryColors[cat] ?? "#6b7280"} 10%, transparent)` }}>
          {cat}
        </span>
      ))}
      <span style={{ flex: 1, fontSize: 12, color: "var(--color-text-muted)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", marginLeft: 8 }}>
        {entry.description}
      </span>
      {!isAdded && (needsOverlayNetwork(entry.urls) ? <OverlayBadge /> : <VerdictBadge health={health} untested={untested} />)}
      {isAdded ? (
        <span style={{ display: "inline-flex", alignItems: "center", gap: 3, fontSize: 10, fontWeight: 600, color: "var(--color-success)", flexShrink: 0 }}>
          <Check size={10} /> Added
        </span>
      ) : (
        <ChevronRight size={16} style={{ color: "var(--color-text-muted)", flexShrink: 0 }} />
      )}
    </button>
  );
}

// Shared badges

export function ProtocolBadge({ protocol }: { protocol: string }) {
  const isTorrent = protocol === "torrent";
  return (
    <span style={{
      display: "inline-flex",
      alignItems: "center",
      gap: 3,
      fontSize: 10,
      fontWeight: 600,
      padding: "2px 6px",
      borderRadius: 3,
      color: isTorrent ? "#3b9eff" : "#f59e0b",
      background: isTorrent
        ? "color-mix(in srgb, #3b9eff 12%, transparent)"
        : "color-mix(in srgb, #f59e0b 12%, transparent)",
      textTransform: "uppercase",
      letterSpacing: "0.04em",
    }}>
      {isTorrent ? <Download size={9} /> : <Newspaper size={9} />}
      {protocol}
    </span>
  );
}

export function PrivacyBadge({ privacy }: { privacy: string }) {
  const colors: Record<string, string> = {
    public: "var(--color-success)",
    "semi-private": "var(--color-warning)",
    private: "var(--color-danger)",
  };
  const color = colors[privacy] ?? "var(--color-text-muted)";
  return (
    <span style={{
      fontSize: 10,
      fontWeight: 500,
      padding: "2px 6px",
      borderRadius: 3,
      color,
      background: `color-mix(in srgb, ${color} 10%, transparent)`,
    }}>
      {privacy}
    </span>
  );
}
