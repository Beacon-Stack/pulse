import PageHeader from "@/components/PageHeader";
import { useSystemStatus } from "@/api/system";
import { card } from "@/lib/styles";
import TableScroll from "@beacon-shared/TableScroll";
import UpdateCheck from "@beacon-shared/UpdateCheck";

export default function SystemPage() {
  const { data: status } = useSystemStatus();

  return (
    <div style={{ padding: 24, maxWidth: 900 }}>
      <PageHeader title="System" description="System information and status." />

      <div style={{ ...card }}>
        <TableScroll minWidth={300}>
        <table style={{ width: "100%", borderCollapse: "collapse" }}>
          <tbody>
            {[
              ["Status", status?.status ?? "-"],
              ["Version", status?.version ?? "-"],
              ["Uptime", status?.uptime ?? "-"],
            ].map(([label, value]) => (
              <tr key={label} style={{ borderBottom: "1px solid var(--color-border-subtle)" }}>
                <td style={{ padding: "10px 0", fontSize: 13, fontWeight: 500, color: "var(--color-text-secondary)", width: 140 }}>{label}</td>
                <td style={{ padding: "10px 0", fontSize: 13, color: "var(--color-text-primary)", fontFamily: "var(--font-family-mono)" }}>{value}</td>
              </tr>
            ))}
            <tr>
              <td style={{ padding: "10px 0", fontSize: 13, fontWeight: 500, color: "var(--color-text-secondary)", width: 140 }}>Updates</td>
              <td style={{ padding: "10px 0" }}>
                <UpdateCheck app="Pulse" service="pulse" image="ghcr.io/beacon-stack/pulse" />
              </td>
            </tr>
          </tbody>
        </table>
        </TableScroll>
      </div>
    </div>
  );
}
