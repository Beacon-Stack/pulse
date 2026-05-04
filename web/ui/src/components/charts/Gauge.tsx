// scaledFontSize returns a font size that keeps `label` inside the ring's
// inner area. The ring is `size` across with stroke ~size/8, so the usable
// inner diameter is roughly `size * 0.7`. We assume monospace-ish digits
// at ~0.62× character width.
function scaledFontSize(label: string, size: number): number {
  const inner = size * 0.7;
  const target = inner / Math.max(label.length * 0.62, 1);
  return Math.min(size / 4.5, Math.max(size / 10, target));
}

interface GaugeProps {
  /** Progress, 0–100. Clamped. */
  value: number;
  /** Diameter in px. Default 64. */
  size?: number;
  /** Track color. */
  trackColor?: string;
  /** Filled portion color. */
  color?: string;
  /** Label centered in the gauge — typically the percentage. */
  label?: string;
  /** Sub-label below the main label. */
  subLabel?: string;
}

/**
 * Ring/donut gauge. Hand-rolled SVG so it accepts CSS variables and is
 * faster than firing up a recharts canvas for a 64×64 element.
 */
export default function Gauge({
  value,
  size = 64,
  trackColor = "var(--color-border-subtle)",
  color = "var(--color-accent)",
  label,
  subLabel,
}: GaugeProps) {
  const clamped = Math.max(0, Math.min(100, value));
  const stroke = Math.max(4, size / 8);
  const radius = (size - stroke) / 2;
  const circumference = 2 * Math.PI * radius;
  const offset = circumference - (clamped / 100) * circumference;

  return (
    <div style={{ position: "relative", width: size, height: size, flexShrink: 0 }}>
      <svg width={size} height={size} style={{ transform: "rotate(-90deg)" }}>
        <circle
          cx={size / 2}
          cy={size / 2}
          r={radius}
          stroke={trackColor}
          strokeWidth={stroke}
          fill="none"
        />
        <circle
          cx={size / 2}
          cy={size / 2}
          r={radius}
          stroke={color}
          strokeWidth={stroke}
          fill="none"
          strokeDasharray={circumference}
          strokeDashoffset={offset}
          strokeLinecap="round"
          style={{ transition: "stroke-dashoffset 300ms ease" }}
        />
      </svg>
      {(label || subLabel) && (
        <div
          style={{
            position: "absolute",
            inset: 0,
            display: "flex",
            flexDirection: "column",
            alignItems: "center",
            justifyContent: "center",
            pointerEvents: "none",
            padding: stroke,
          }}
        >
          {label && (
            <div
              style={{
                // Scale font down for longer labels so things like "5.2 MB/s"
                // stay inside the ring instead of overflowing past the stroke.
                fontSize: scaledFontSize(label, size),
                fontWeight: 600,
                color: "var(--color-text-primary)",
                lineHeight: 1,
                fontVariantNumeric: "tabular-nums",
                whiteSpace: "nowrap",
              }}
            >
              {label}
            </div>
          )}
          {subLabel && (
            <div
              style={{
                fontSize: size / 9,
                color: "var(--color-text-muted)",
                marginTop: 2,
                whiteSpace: "nowrap",
              }}
            >
              {subLabel}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
