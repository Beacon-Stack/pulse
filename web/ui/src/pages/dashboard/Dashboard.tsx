import { Server, Search, Gauge, Activity } from "lucide-react";
import { useServices } from "@/api/services";
import { useIndexers } from "@/api/indexers";
import { useQualityProfiles } from "@/api/quality-profiles";
import { useSystemStatus } from "@/api/system";
import { card } from "@/lib/styles";
import StatusBadge from "@/components/StatusBadge";
import { timeAgo } from "@/lib/utils";

function StatCard({ icon: Icon, label, value, color }: { icon: React.ElementType; label: string; value: string | number; color: string }) {
  return (
    <div style={{ ...card, display: "flex", alignItems: "center", gap: 16 }}>
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
        <div style={{ fontSize: 24, fontWeight: 700, color: "var(--color-text-primary)", lineHeight: 1 }}>{value}</div>
        <div style={{ fontSize: 13, color: "var(--color-text-secondary)", marginTop: 2 }}>{label}</div>
      </div>
    </div>
  );
}

export default function Dashboard() {
  const { data: services } = useServices();
  const { data: indexers } = useIndexers();
  const { data: qualityProfiles } = useQualityProfiles();
  const { data: status } = useSystemStatus();

  const onlineCount = services?.filter((s) => s.status === "online").length ?? 0;
  const enabledIndexers = indexers?.filter((i) => i.enabled).length ?? 0;

  return (
    <div style={{ padding: 24, maxWidth: 1200 }}>
      <h1 style={{ margin: "0 0 4px", fontSize: 20, fontWeight: 600, color: "var(--color-text-primary)" }}>
        Dashboard
      </h1>
      <p style={{ margin: "0 0 24px", fontSize: 13, color: "var(--color-text-secondary)" }}>
        Centralized control plane for the Arr ecosystem
        {status && <span style={{ marginLeft: 8, color: "var(--color-text-muted)" }}>Uptime: {status.uptime}</span>}
      </p>

      {/* Stats */}
      <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(220px, 1fr))", gap: 16, marginBottom: 32 }}>
        <StatCard icon={Server} label="Services" value={services?.length ?? 0} color="var(--color-accent)" />
        <StatCard icon={Activity} label="Online" value={onlineCount} color="var(--color-success)" />
        <StatCard icon={Search} label="Indexers" value={enabledIndexers} color="var(--color-info)" />
        <StatCard icon={Gauge} label="Quality profiles" value={qualityProfiles?.length ?? 0} color="var(--color-warning)" />
      </div>

      {/* Recent services */}
      <h2 style={{ margin: "0 0 12px", fontSize: 14, fontWeight: 600, color: "var(--color-text-primary)" }}>
        Registered Services
      </h2>
      {!services?.length ? (
        <div style={{ ...card, color: "var(--color-text-muted)", fontSize: 13, textAlign: "center", padding: 40 }}>
          No services registered yet. Services will appear here once they register with Pulse.
        </div>
      ) : (
        <div style={{ ...card, padding: 0, overflow: "hidden" }}>
          <table style={{ width: "100%", borderCollapse: "collapse" }}>
            <thead>
              <tr style={{ borderBottom: "1px solid var(--color-border-subtle)" }}>
                {["Name", "Type", "Status", "Version", "Last Seen"].map((h) => (
                  <th
                    key={h}
                    style={{
                      textAlign: "left",
                      padding: "10px 16px",
                      fontSize: 11,
                      fontWeight: 600,
                      color: "var(--color-text-muted)",
                      textTransform: "uppercase",
                      letterSpacing: "0.06em",
                    }}
                  >
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {services.map((svc) => (
                <tr
                  key={svc.id}
                  style={{ borderBottom: "1px solid var(--color-border-subtle)" }}
                >
                  <td style={{ padding: "10px 16px", fontSize: 14, fontWeight: 500, color: "var(--color-text-primary)" }}>
                    {svc.name}
                  </td>
                  <td style={{ padding: "10px 16px", fontSize: 13, color: "var(--color-text-secondary)" }}>
                    {svc.type}
                  </td>
                  <td style={{ padding: "10px 16px" }}>
                    <StatusBadge status={svc.status} />
                  </td>
                  <td style={{ padding: "10px 16px", fontSize: 13, color: "var(--color-text-secondary)", fontFamily: "var(--font-family-mono)" }}>
                    {svc.version || "-"}
                  </td>
                  <td style={{ padding: "10px 16px", fontSize: 13, color: "var(--color-text-muted)" }}>
                    {svc.last_seen ? timeAgo(svc.last_seen) : "-"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
