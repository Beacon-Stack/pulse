import { Shield, ShieldOff, Globe, Lock } from "lucide-react";
import type { VPNSummary } from "@/api/dashboard";
import { card } from "@/lib/styles";

interface VPNPanelProps {
  vpn: VPNSummary;
}

/**
 * VPN status panel. Hidden entirely (parent renders null) when the
 * Gluetun client is not configured. When connected, shows public IP,
 * country, port-forwarded state, and DNS status.
 */
export default function VPNPanel({ vpn }: VPNPanelProps) {
  const Icon = vpn.connected ? Shield : ShieldOff;
  const accent = vpn.connected
    ? "var(--color-success)"
    : "var(--color-status-offline)";

  return (
    <div style={{ ...card, padding: 16 }}>
      <div style={{ display: "flex", alignItems: "center", gap: 10, marginBottom: 12 }}>
        <div
          style={{
            width: 32,
            height: 32,
            borderRadius: 8,
            background: `color-mix(in srgb, ${accent} 12%, transparent)`,
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
          }}
        >
          <Icon size={16} style={{ color: accent }} strokeWidth={1.75} />
        </div>
        <div style={{ flex: 1 }}>
          <div
            style={{
              fontSize: 13,
              fontWeight: 600,
              color: "var(--color-text-primary)",
            }}
          >
            VPN {vpn.connected ? "connected" : "disconnected"}
          </div>
          <div style={{ fontSize: 12, color: "var(--color-text-muted)" }}>
            {vpn.provider || (vpn.country ? `via ${vpn.country}` : "tunnel up")}
          </div>
        </div>
      </div>

      <Row icon={<Globe size={14} />} label="Public IP" value={vpn.public_ip || "—"} />
      <Row label="Country" value={vpn.country || "—"} indent />
      <Row
        icon={<Lock size={14} />}
        label="Port forwarded"
        value={vpn.port_forwarded ? String(vpn.port_forwarded) : "no"}
      />
      <Row label="DNS" value={vpn.dns_status || "—"} indent />
    </div>
  );
}

function Row({
  icon,
  label,
  value,
  indent,
}: {
  icon?: React.ReactNode;
  label: string;
  value: string;
  indent?: boolean;
}) {
  return (
    <div
      style={{
        display: "flex",
        alignItems: "center",
        gap: 6,
        fontSize: 12,
        padding: "4px 0",
        color: "var(--color-text-secondary)",
        paddingLeft: indent ? 20 : 0,
      }}
    >
      {icon}
      <span>{label}</span>
      <span style={{ marginLeft: "auto", color: "var(--color-text-primary)", fontFamily: "var(--font-family-mono)" }}>
        {value}
      </span>
    </div>
  );
}
