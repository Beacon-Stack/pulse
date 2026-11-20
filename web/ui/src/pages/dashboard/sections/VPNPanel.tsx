import { useMemo, useState } from "react";
import { Shield, ShieldOff, Globe, Lock, RotateCw, MapPin, AlertTriangle } from "lucide-react";
import Modal from "@beacon-shared/Modal";
import { useConfirm } from "@beacon-shared/ConfirmDialog";
import { useRotateVPN, useVPNAdvice, useVPNRegions, useChangeVPNRegion } from "@/api/dashboard";
import type { VPNSummary, VPNAdvice, VPNRegions } from "@/api/dashboard";
import { card } from "@/lib/styles";

interface VPNPanelProps {
  vpn: VPNSummary;
}

/**
 * VPN status panel. Hidden entirely (parent renders null) when the
 * Gluetun client is not configured. Shows public IP, country, region,
 * port forwarding and DNS, with two actions for shaking off Cloudflare:
 * rotate to a new IP, or move to another region. When FlareSolverr keeps
 * failing Cloudflare challenges from this IP, the panel says so and
 * highlights Rotate.
 */
export default function VPNPanel({ vpn }: VPNPanelProps) {
  const Icon = vpn.connected ? Shield : ShieldOff;
  const accent = vpn.connected ? "var(--color-success)" : "var(--color-status-offline)";
  const rotate = useRotateVPN();
  const changeRegion = useChangeVPNRegion();
  const confirm = useConfirm();
  const advice = useVPNAdvice(vpn.connected).data;
  const regions = useVPNRegions(vpn.connected).data;
  const [picking, setPicking] = useState(false);
  const busy = rotate.isPending || changeRegion.isPending;
  const recommend = !!advice?.recommend_rotate;
  const region = regions?.current?.length === 1 ? regions.current[0] : regions?.current?.join(", ");

  async function handleRotate() {
    if (
      !(await confirm({
        title: "Rotate VPN IP",
        message:
          "The tunnel reconnects until the provider assigns a new exit IP (up to ~2 minutes). Active downloads pause briefly and all Cloudflare sessions are re-solved.",
        confirmLabel: "Rotate",
      }))
    )
      return;
    rotate.mutate();
  }

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
          <div style={{ fontSize: 13, fontWeight: 600, color: "var(--color-text-primary)" }}>
            VPN {vpn.connected ? "connected" : "disconnected"}
          </div>
          <div style={{ fontSize: 12, color: "var(--color-text-muted)" }}>
            {vpn.provider || (vpn.country ? `via ${vpn.country}` : "tunnel up")}
          </div>
        </div>
      </div>

      {recommend && advice && <RotateAdvice advice={advice} />}

      <Row icon={<Globe size={14} />} label="Public IP" value={vpn.public_ip || "—"} />
      <Row label="Country" value={vpn.country || "—"} indent />
      {region && <Row label="Region" value={region} indent />}
      <Row
        icon={<Lock size={14} />}
        label="Port forwarded"
        value={vpn.port_forwarded ? String(vpn.port_forwarded) : "no"}
      />
      <Row label="DNS" value={vpn.dns_status || "—"} indent />

      {vpn.connected && (
        <div style={{ display: "flex", gap: 8, marginTop: 12 }}>
          <ActionButton
            onClick={handleRotate}
            disabled={busy}
            spinning={rotate.isPending}
            highlight={recommend}
            icon={<RotateCw size={13} />}
            label={rotate.isPending ? "Rotating…" : "Rotate IP"}
            title="Reconnect the tunnel until the provider assigns a new exit IP"
          />
          <ActionButton
            onClick={() => setPicking(true)}
            disabled={busy || !regions?.available}
            spinning={changeRegion.isPending}
            icon={<MapPin size={13} />}
            label={changeRegion.isPending ? "Moving…" : "Change region"}
            title={regions?.available ? "Move the VPN to another region" : (regions?.reason ?? "Loading regions…")}
          />
        </div>
      )}

      {picking && regions?.available && (
        <RegionPicker
          regions={regions}
          onClose={() => setPicking(false)}
          onPick={(r) => {
            setPicking(false);
            changeRegion.mutate(r);
          }}
        />
      )}
    </div>
  );
}

function RotateAdvice({ advice }: { advice: VPNAdvice }) {
  const failures = advice.sites.reduce((n, s) => n + s.failures, 0);
  const hours = Math.round(advice.window_minutes / 60);
  const names = advice.sites.map((s) => s.host).join(", ");
  return (
    <div
      role="status"
      style={{
        display: "flex",
        gap: 8,
        alignItems: "flex-start",
        padding: "8px 10px",
        marginBottom: 10,
        borderRadius: 6,
        fontSize: 12,
        lineHeight: 1.45,
        color: "var(--color-text-primary)",
        background: "color-mix(in srgb, var(--color-warning) 10%, transparent)",
        border: "1px solid color-mix(in srgb, var(--color-warning) 35%, transparent)",
      }}
    >
      <AlertTriangle size={14} style={{ color: "var(--color-warning)", flexShrink: 0, marginTop: 1 }} />
      <span title={advice.sites.map((s) => `${s.host}: ${s.last_error}`).join("\n")}>
        Cloudflare keeps blocking {names} from this IP ({failures} failed in the last {hours} hours). Rotating the IP or
        changing region usually fixes it.
      </span>
    </div>
  );
}

function RegionPicker({
  regions,
  onClose,
  onPick,
}: {
  regions: VPNRegions;
  onClose: () => void;
  onPick: (region: string) => void;
}) {
  const [query, setQuery] = useState("");
  const [chosen, setChosen] = useState<string | null>(null);
  const current = new Set(regions.current);
  const shown = useMemo(() => {
    const q = query.trim().toLowerCase();
    return q ? regions.regions.filter((r) => r.toLowerCase().includes(q)) : regions.regions;
  }, [query, regions.regions]);

  return (
    <Modal onClose={onClose} width={440}>
      <div style={{ padding: 20, display: "flex", flexDirection: "column", gap: 12 }}>
        <h2 style={{ margin: 0, fontSize: 16, fontWeight: 600, color: "var(--color-text-primary)" }}>
          Change VPN region
        </h2>
        <p style={{ margin: 0, fontSize: 12, color: "var(--color-text-secondary)", lineHeight: 1.5 }}>
          Now: {regions.current.join(", ") || "any"}. Downloads pause for a few seconds while the VPN reconnects. Pulse
          keeps your choice if the VPN restarts.
          {regions.port_forward_only && " Only regions that support port forwarding are listed."}
        </p>
        <input
          autoFocus
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder={`Search ${regions.regions.length} regions`}
          aria-label="Search regions"
          style={{
            padding: "7px 10px",
            borderRadius: 6,
            border: "1px solid var(--color-border-default)",
            background: "var(--color-bg-base)",
            color: "var(--color-text-primary)",
            fontSize: 13,
          }}
        />
        <div
          role="listbox"
          aria-label="Regions"
          style={{
            maxHeight: 300,
            overflowY: "auto",
            border: "1px solid var(--color-border-subtle)",
            borderRadius: 6,
          }}
        >
          {shown.map((r) => {
            const isCurrent = current.has(r);
            const isChosen = chosen === r;
            return (
              <button
                key={r}
                role="option"
                aria-selected={isChosen}
                disabled={isCurrent}
                onClick={() => setChosen(r)}
                style={{
                  display: "flex",
                  width: "100%",
                  justifyContent: "space-between",
                  padding: "7px 10px",
                  border: "none",
                  borderBottom: "1px solid var(--color-border-subtle)",
                  background: isChosen ? "var(--color-accent-muted)" : "transparent",
                  color: isCurrent ? "var(--color-text-muted)" : "var(--color-text-primary)",
                  fontSize: 13,
                  textAlign: "left",
                  cursor: isCurrent ? "default" : "pointer",
                }}
              >
                <span>{r}</span>
                {isCurrent && <span style={{ fontSize: 11 }}>current</span>}
              </button>
            );
          })}
          {shown.length === 0 && (
            <div style={{ padding: 12, fontSize: 12, color: "var(--color-text-muted)" }}>No region matches.</div>
          )}
        </div>
        <div style={{ display: "flex", justifyContent: "flex-end", gap: 8 }}>
          <button
            onClick={onClose}
            style={{
              padding: "7px 16px",
              borderRadius: 6,
              border: "1px solid var(--color-border-default)",
              background: "transparent",
              color: "var(--color-text-secondary)",
              fontSize: 13,
              cursor: "pointer",
            }}
          >
            Cancel
          </button>
          <button
            onClick={() => chosen && onPick(chosen)}
            disabled={!chosen}
            style={{
              padding: "7px 16px",
              borderRadius: 6,
              border: "none",
              background: "var(--color-accent)",
              color: "var(--color-accent-fg)",
              fontSize: 13,
              fontWeight: 500,
              cursor: chosen ? "pointer" : "default",
              opacity: chosen ? 1 : 0.5,
            }}
          >
            {chosen ? `Move to ${chosen}` : "Pick a region"}
          </button>
        </div>
      </div>
    </Modal>
  );
}

function ActionButton({
  onClick,
  disabled,
  spinning,
  highlight,
  icon,
  label,
  title,
}: {
  onClick: () => void;
  disabled: boolean;
  spinning: boolean;
  highlight?: boolean;
  icon: React.ReactNode;
  label: string;
  title: string;
}) {
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      title={title}
      style={{
        display: "flex",
        alignItems: "center",
        gap: 5,
        padding: "5px 10px",
        borderRadius: 6,
        border: highlight
          ? "1px solid var(--color-warning)"
          : "1px solid var(--color-border-default)",
        background: highlight ? "color-mix(in srgb, var(--color-warning) 14%, transparent)" : "transparent",
        color: highlight ? "var(--color-text-primary)" : "var(--color-text-secondary)",
        fontSize: 12,
        fontWeight: highlight ? 600 : 500,
        cursor: spinning ? "wait" : disabled ? "default" : "pointer",
        opacity: disabled && !spinning ? 0.55 : 1,
      }}
    >
      <span style={spinning ? { display: "flex", animation: "spin 1s linear infinite" } : { display: "flex" }}>
        {icon}
      </span>
      {label}
    </button>
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
