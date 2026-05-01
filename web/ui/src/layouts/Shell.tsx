import { useEffect } from "react";
import {
  Activity,
  Cog,
  Download,
  FolderCog,
  Gauge,
  LayoutDashboard,
  Network,
  Paintbrush,
  Search,
  Server,
} from "lucide-react";
import Shell, { type NavItem } from "@beacon-shared/Shell";
import { useSystemStatus } from "@/api/system";
import { useWebSocket } from "@/api/websocket";
import { applyTheme } from "@/theme";

const mainNav: NavItem[] = [
  { to: "/", icon: LayoutDashboard, label: "Dashboard" },
  { to: "/services", icon: Server, label: "Services" },
  { to: "/indexers", icon: Search, label: "Indexers" },
  { to: "/download-clients", icon: Download, label: "Download Clients" },
  { to: "/quality-profiles", icon: Gauge, label: "Quality Profiles" },
  { to: "/shared-settings", icon: FolderCog, label: "Shared Settings" },
];

const settingsNav: NavItem[] = [
  { to: "/settings/system", icon: Cog, label: "System" },
  { to: "/settings/app", icon: Paintbrush, label: "App Settings" },
];

// HealthDot reads /api/system/status and renders a colored dot + label
// in the sidebar footer. Lives in this file (not web-shared) because
// it's bound to Pulse's status API contract.
function HealthDot() {
  const { data: status } = useSystemStatus();
  const ok = !status || status.status === "ok";
  const color = ok ? "var(--color-success)" : "var(--color-danger)";
  const label = ok ? "System healthy" : "Issues detected";

  return (
    <div
      style={{
        display: "flex",
        alignItems: "center",
        gap: "8px",
        padding: "0 12px",
        height: "36px",
        color: "var(--color-text-muted)",
        fontSize: "12px",
      }}
      title={label}
    >
      <Activity
        size={16}
        strokeWidth={1.5}
        style={{ color, flexShrink: 0 }}
      />
      <span style={{ color }}>{label}</span>
    </div>
  );
}

// AppIcon wraps the Network glyph in Pulse's accent-color tile.
// Web-shared's Shell accepts the icon as a fully-rendered ReactNode
// so each app keeps its own visual identity.
function AppIcon() {
  return (
    <div
      style={{
        width: 32,
        height: 32,
        borderRadius: "8px",
        background: "var(--color-accent)",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        flexShrink: 0,
      }}
    >
      <Network size={18} color="white" strokeWidth={2} />
    </div>
  );
}

export default function PulseShell() {
  useWebSocket();
  useEffect(() => {
    applyTheme();
  }, []);

  return (
    <Shell
      appName="Pulse"
      appIcon={<AppIcon />}
      mainNav={mainNav}
      settingsNav={settingsNav}
      collapsedStorageKey="sidebar-collapsed"
      sidebarFooterExtras={<HealthDot />}
    />
  );
}
