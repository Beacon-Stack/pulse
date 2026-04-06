import PageHeader from "@/components/PageHeader";
import { useSystemStatus } from "@/api/system";
import { card } from "@/lib/styles";

export default function SystemPage() {
  const { data: status } = useSystemStatus();

  return (
    <div style={{ padding: 24, maxWidth: 900 }}>
      <PageHeader title="System" description="System information and status." />

      <div style={{ ...card }}>
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
          </tbody>
        </table>
      </div>
    </div>
  );
}
