import { useSearchParams } from "react-router-dom";
import { useDashboardOverview } from "@/api/dashboard";
import { useServices } from "@/api/services";
import { useSystemStatus } from "@/api/system";
import { card } from "@/lib/styles";
import { timeAgo } from "@/lib/utils";
import StatusBadge from "@/components/StatusBadge";
import TableScroll from "@beacon-shared/TableScroll";
import TopStats from "./sections/TopStats";
import VPNPanel from "./sections/VPNPanel";
import ThroughputPanel from "./sections/ThroughputPanel";
import ServicesGrid from "./sections/ServicesGrid";
import DownloadsPanel from "./sections/DownloadsPanel";
import ImportsPanel from "./sections/ImportsPanel";
import ServiceDrawer from "./drilldown/ServiceDrawer";

export default function Dashboard() {
  const [params, setParams] = useSearchParams();
  const selected = params.get("service");

  const { data: overview } = useDashboardOverview();
  const { data: services } = useServices();
  const { data: status } = useSystemStatus();

  return (
    <div style={{ padding: 24, maxWidth: 1400 }}>
      <h1
        style={{
          margin: "0 0 4px",
          fontSize: 20,
          fontWeight: 600,
          color: "var(--color-text-primary)",
        }}
      >
        Dashboard
      </h1>
      <p
        style={{
          margin: "0 0 20px",
          fontSize: 13,
          color: "var(--color-text-secondary)",
        }}
      >
        Centralized control plane for the Arr ecosystem
        {status && (
          <span style={{ marginLeft: 8, color: "var(--color-text-muted)" }}>
            Uptime: {status.uptime}
          </span>
        )}
      </p>

      <TopStats />

      {/* Side-by-side VPN + throughput row. Each panel hides when its
         data source isn't available — if both are off, this row collapses
         to nothing rather than showing empty placeholders. */}
      {(overview?.vpn || overview?.haul) && (
        <div
          style={{
            display: "grid",
            gridTemplateColumns: "repeat(auto-fit, minmax(280px, 1fr))",
            gap: 16,
            marginBottom: 16,
          }}
        >
          {overview?.vpn && <VPNPanel vpn={overview.vpn} />}
          {overview?.haul && <ThroughputPanel haul={overview.haul} />}
        </div>
      )}

      {/* Active feeds — the "what's happening right now" view across the
         whole stack. Each panel hides itself when its total is 0, so a
         quiet stack collapses to nothing. */}
      <DownloadsPanel
        rows={overview?.active_downloads ?? []}
        total={overview?.active_downloads_total ?? 0}
      />
      <div style={{ height: 16 }} />
      <ImportsPanel
        rows={overview?.active_imports ?? []}
        total={overview?.active_imports_total ?? 0}
      />

      <h2
        style={{
          margin: "24px 0 12px",
          fontSize: 14,
          fontWeight: 600,
          color: "var(--color-text-primary)",
        }}
      >
        Services
      </h2>

      {overview?.services && <ServicesGrid services={overview.services} />}

      {/* Registered services table — preserved from the original dashboard.
         Still useful for ops (last_seen, version), and the new cards above
         don't fully replace it because they only show services that have
         live container stats. */}
      <h2
        style={{
          margin: "24px 0 12px",
          fontSize: 14,
          fontWeight: 600,
          color: "var(--color-text-primary)",
        }}
      >
        Registered Services
      </h2>
      {!services?.length ? (
        <div
          style={{
            ...card,
            color: "var(--color-text-muted)",
            fontSize: 13,
            textAlign: "center",
            padding: 40,
          }}
        >
          No services registered yet. Services will appear here once they register with Pulse.
        </div>
      ) : (
        <div style={{ ...card, padding: 0, overflow: "hidden" }}>
          <TableScroll minWidth={700}>
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
                <tr key={svc.id} style={{ borderBottom: "1px solid var(--color-border-subtle)" }}>
                  <td
                    style={{
                      padding: "10px 16px",
                      fontSize: 14,
                      fontWeight: 500,
                      color: "var(--color-text-primary)",
                    }}
                  >
                    {svc.name}
                  </td>
                  <td
                    style={{
                      padding: "10px 16px",
                      fontSize: 13,
                      color: "var(--color-text-secondary)",
                    }}
                  >
                    {svc.type}
                  </td>
                  <td style={{ padding: "10px 16px" }}>
                    <StatusBadge status={svc.status} />
                  </td>
                  <td
                    style={{
                      padding: "10px 16px",
                      fontSize: 13,
                      color: "var(--color-text-secondary)",
                      fontFamily: "var(--font-family-mono)",
                    }}
                  >
                    {svc.version || "-"}
                  </td>
                  <td
                    style={{
                      padding: "10px 16px",
                      fontSize: 13,
                      color: "var(--color-text-muted)",
                    }}
                  >
                    {svc.last_seen ? timeAgo(svc.last_seen) : "-"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          </TableScroll>
        </div>
      )}

      {selected && (
        <ServiceDrawer
          serviceId={selected}
          onClose={() => {
            setParams(
              (prev) => {
                const next = new URLSearchParams(prev);
                next.delete("service");
                return next;
              },
              { replace: false },
            );
          }}
        />
      )}
    </div>
  );
}
