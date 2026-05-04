import { LineChart, Line, ResponsiveContainer } from "recharts";

interface SparklineProps {
  /** Recent values, oldest first. */
  data: number[];
  /** CSS color expression — supports `var(--color-*)`. */
  color?: string;
  height?: number;
}

/**
 * Small inline trend line. The parent maintains the buffer of recent
 * samples (typically the last 10 polls × 2s = 20s of history); this just
 * draws them. No axes, no tooltips.
 */
export default function Sparkline({
  data,
  color = "var(--color-accent)",
  height = 24,
}: SparklineProps) {
  // recharts wants {value} entries.
  const chartData = data.map((v, i) => ({ i, value: v }));

  return (
    <div style={{ width: "100%", height }}>
      <ResponsiveContainer>
        <LineChart data={chartData} margin={{ top: 2, right: 2, bottom: 2, left: 2 }}>
          <Line
            type="monotone"
            dataKey="value"
            stroke={color}
            strokeWidth={1.5}
            dot={false}
            isAnimationActive={false}
          />
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
}
