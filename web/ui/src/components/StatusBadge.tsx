const statusColors: Record<string, string> = {
  online: "var(--color-status-online)",
  offline: "var(--color-status-offline)",
  degraded: "var(--color-status-degraded)",
  unknown: "var(--color-status-unknown)",
};

export default function StatusBadge({ status }: { status: string }) {
  const color = statusColors[status] ?? statusColors.unknown;
  return (
    <span
      style={{
        display: "inline-flex",
        alignItems: "center",
        gap: 6,
        padding: "2px 10px",
        borderRadius: 4,
        fontSize: 12,
        fontWeight: 500,
        color,
        background: `color-mix(in srgb, ${color} 12%, transparent)`,
      }}
    >
      <span
        style={{
          width: 6,
          height: 6,
          borderRadius: "50%",
          background: color,
        }}
      />
      {status}
    </span>
  );
}
