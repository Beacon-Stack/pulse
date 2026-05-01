import { Server, Search, Gauge, Activity } from "lucide-react";
import { Link } from "react-router-dom";
import { useServices } from "@/api/services";
import { useIndexers } from "@/api/indexers";
import { useQualityProfiles } from "@/api/quality-profiles";
import { card } from "@/lib/styles";

function StatCard({
  icon: Icon,
  label,
  value,
  color,
  to,
}: {
  icon: React.ElementType;
  label: string;
  value: string | number;
  color: string;
  to: string;
}) {
  return (
    <Link
      to={to}
      style={{
        ...card,
        display: "flex",
        alignItems: "center",
        gap: 16,
        textDecoration: "none",
        cursor: "pointer",
        transition: "border-color 150ms",
      }}
      onMouseEnter={(e) => (e.currentTarget.style.borderColor = "var(--color-border-strong)")}
      onMouseLeave={(e) => (e.currentTarget.style.borderColor = "var(--color-border-subtle)")}
    >
      <div
        style={{
          width: 44,
          height: 44,
          borderRadius: 10,
          background: `color-mix(in srgb, ${color} 12%, transparent)`,
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          flexShrink: 0,
        }}
      >
        <Icon size={22} style={{ color }} strokeWidth={1.5} />
      </div>
      <div>
        <div
          style={{
            fontSize: 24,
            fontWeight: 700,
            color: "var(--color-text-primary)",
            lineHeight: 1,
          }}
        >
          {value}
        </div>
        <div
          style={{ fontSize: 13, color: "var(--color-text-secondary)", marginTop: 2 }}
        >
          {label}
        </div>
      </div>
    </Link>
  );
}

/**
 * The four top-level counts. Preserved from the original dashboard since
 * the user explicitly liked them — services / online / indexers / profiles.
 */
export default function TopStats() {
  const { data: services } = useServices();
  const { data: indexers } = useIndexers();
  const { data: qualityProfiles } = useQualityProfiles();

  const onlineCount = services?.filter((s) => s.status === "online").length ?? 0;
  const enabledIndexers = indexers?.filter((i) => i.enabled).length ?? 0;

  return (
    <div
      style={{
        display: "grid",
        gridTemplateColumns: "repeat(auto-fit, minmax(220px, 1fr))",
        gap: 16,
        marginBottom: 16,
      }}
    >
      <StatCard
        icon={Server}
        label="Services"
        value={services?.length ?? 0}
        color="var(--color-accent)"
        to="/services"
      />
      <StatCard
        icon={Activity}
        label="Online"
        value={onlineCount}
        color="var(--color-success)"
        to="/services"
      />
      <StatCard
        icon={Search}
        label="Indexers"
        value={enabledIndexers}
        color="var(--color-info)"
        to="/indexers"
      />
      <StatCard
        icon={Gauge}
        label="Quality profiles"
        value={qualityProfiles?.length ?? 0}
        color="var(--color-warning)"
        to="/quality-profiles"
      />
    </div>
  );
}
