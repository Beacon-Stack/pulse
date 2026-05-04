interface MiniBarProps {
  /** 0–1, where 1 fills the bar. Clamped. */
  value: number;
  color?: string;
  trackColor?: string;
  height?: number;
}

/**
 * Slim horizontal progress bar. Used for paired up/down throughput
 * indicators where stacking two bars conveys "more like this" without
 * needing axis labels.
 */
export default function MiniBar({
  value,
  color = "var(--color-accent)",
  trackColor = "var(--color-border-subtle)",
  height = 6,
}: MiniBarProps) {
  const pct = Math.max(0, Math.min(1, value)) * 100;
  return (
    <div
      style={{
        width: "100%",
        height,
        background: trackColor,
        borderRadius: height,
        overflow: "hidden",
      }}
    >
      <div
        style={{
          width: `${pct}%`,
          height: "100%",
          background: color,
          transition: "width 300ms ease",
        }}
      />
    </div>
  );
}
