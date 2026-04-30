import { useState, useEffect } from "react";
import { Link, NavLink, Outlet, useLocation } from "react-router-dom";
import {
  LayoutDashboard,
  Server,
  Search,
  Download,
  Cog,
  ChevronLeft,
  ChevronRight,
  Menu,
  X,
  Activity,
  Network,
  Paintbrush,
  Gauge,
  FolderCog,
} from "lucide-react";
import { useWebSocket } from "@/api/websocket";
import { useSystemStatus } from "@/api/system";
import { applyTheme } from "@/theme";

interface NavItem {
  to: string;
  icon: React.ElementType;
  label: string;
}

const mainNav: NavItem[] = [
  { to: "/",         icon: LayoutDashboard, label: "Dashboard" },
  { to: "/services", icon: Server,          label: "Services" },
  { to: "/indexers",          icon: Search,   label: "Indexers" },
  { to: "/download-clients", icon: Download, label: "Download Clients" },
  { to: "/quality-profiles", icon: Gauge,    label: "Quality Profiles" },
  { to: "/shared-settings",  icon: FolderCog, label: "Shared Settings" },
];

const settingsNav: NavItem[] = [
  { to: "/settings/system", icon: Cog,        label: "System" },
  { to: "/settings/app",    icon: Paintbrush,  label: "App Settings" },
];

// Viewport tiers, in order from narrowest to widest:
//
//   mobile   <768px   slide-out drawer + hamburger top bar
//   compact  768–1100 sidebar force-collapsed to 60px icons-only
//   wide     ≥1100px  sidebar honors the user's saved expanded/collapsed pref
//
// The compact tier exists because the right pane needs at least ~1024px
// to render most settings forms and tables comfortably; a 240px sidebar
// at 1024-1100px viewport leaves only ~700px and crushes the content.
type ViewportMode = "mobile" | "compact" | "wide";

function computeViewportMode(): ViewportMode {
  if (typeof window === "undefined") return "wide";
  if (window.innerWidth < 768) return "mobile";
  if (window.innerWidth < 1100) return "compact";
  return "wide";
}

function useViewportMode(): ViewportMode {
  const [mode, setMode] = useState<ViewportMode>(computeViewportMode);
  useEffect(() => {
    const handler = () => setMode(computeViewportMode());
    const mqMobile = window.matchMedia("(max-width: 767px)");
    const mqCompact = window.matchMedia("(max-width: 1099px)");
    mqMobile.addEventListener("change", handler);
    mqCompact.addEventListener("change", handler);
    return () => {
      mqMobile.removeEventListener("change", handler);
      mqCompact.removeEventListener("change", handler);
    };
  }, []);
  return mode;
}

function SidebarNavItem({
  item,
  collapsed,
  onClick,
}: {
  item: NavItem;
  collapsed: boolean;
  onClick?: () => void;
}) {
  const Icon = item.icon;
  return (
    <NavLink
      to={item.to}
      end={item.to === "/"}
      // Always set the title so the full label is reachable on hover
      // even when the rail shows the text — long labels can still
      // ellipsis-clip at narrow widths and the tooltip is the recovery.
      title={item.label}
      onClick={onClick}
      style={({ isActive }) => ({
        display: "flex",
        alignItems: "center",
        gap: "10px",
        padding: "0 12px",
        height: "40px",
        borderRadius: "6px",
        textDecoration: "none",
        fontSize: "14px",
        fontWeight: 500,
        whiteSpace: "nowrap",
        overflow: "hidden",
        transition: "background 150ms ease, color 150ms ease",
        borderLeft: isActive ? "2px solid var(--color-accent)" : "2px solid transparent",
        background: isActive ? "var(--color-accent-muted)" : "transparent",
        color: isActive ? "var(--color-accent-hover)" : "var(--color-text-secondary)",
        marginLeft: "-2px",
      })}
    >
      <Icon size={18} strokeWidth={1.5} style={{ flexShrink: 0 }} />
      {!collapsed && (
        <span
          style={{
            // Soft-clip with ellipsis instead of hard cut — without
            // this, long labels rendered mid-word with no visual cue.
            overflow: "hidden",
            textOverflow: "ellipsis",
            minWidth: 0,
          }}
        >
          {item.label}
        </span>
      )}
    </NavLink>
  );
}

function HealthDot({ collapsed }: { collapsed: boolean }) {
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
      title={collapsed ? label : undefined}
    >
      <Activity size={16} strokeWidth={1.5} style={{ color, flexShrink: 0 }} />
      {!collapsed && <span style={{ color }}>{label}</span>}
    </div>
  );
}

function Sidebar({
  collapsed,
  onCollapse,
  onClose,
  isMobile,
  autoCollapsed,
}: {
  collapsed: boolean;
  onCollapse: () => void;
  onClose: () => void;
  isMobile: boolean;
  // autoCollapsed=true means the viewport forced compact mode. The
  // toggle button is hidden in that case — manual override would
  // bounce back on the next render anyway.
  autoCollapsed?: boolean;
}) {
  const width = isMobile ? 240 : collapsed ? 60 : 240;

  return (
    <nav
      style={{
        width,
        minWidth: width,
        maxWidth: width,
        background: "var(--color-bg-surface)",
        borderRight: "1px solid var(--color-border-subtle)",
        display: "flex",
        flexDirection: "column",
        transition: "width 200ms ease, min-width 200ms ease, max-width 200ms ease",
        overflow: "hidden",
        position: "fixed",
        top: 0,
        left: 0,
        height: "100vh",
        zIndex: 50,
      }}
    >
      {/* Logo */}
      <div
        style={{
          height: "60px",
          display: "flex",
          alignItems: "center",
          padding: "0 14px",
          borderBottom: "1px solid var(--color-border-subtle)",
          flexShrink: 0,
        }}
      >
        <Link
          to="/"
          style={{ display: "flex", alignItems: "center", textDecoration: "none" }}
        >
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
          {(!collapsed || isMobile) && (
            <span
              style={{
                marginLeft: "10px",
                fontSize: "16px",
                fontWeight: 700,
                color: "var(--color-text-primary)",
                letterSpacing: "-0.01em",
                whiteSpace: "nowrap",
                flex: 1,
              }}
            >
              Pulse
            </span>
          )}
        </Link>
        {isMobile && (
          <button
            onClick={onClose}
            style={{
              background: "none",
              border: "none",
              cursor: "pointer",
              color: "var(--color-text-muted)",
              display: "flex",
              alignItems: "center",
              padding: 4,
              marginLeft: "auto",
            }}
          >
            <X size={18} />
          </button>
        )}
      </div>

      {/* Nav items */}
      <div
        style={{
          flex: 1,
          overflowY: "auto",
          overflowX: "hidden",
          padding: "12px 8px",
          display: "flex",
          flexDirection: "column",
          gap: "2px",
        }}
      >
        {mainNav.map((item) => (
          <SidebarNavItem
            key={item.to}
            item={item}
            collapsed={!isMobile && collapsed}
            onClick={isMobile ? onClose : undefined}
          />
        ))}

        <div
          style={{
            // Margins must collapse to 0 alongside height so the band
            // disappears entirely when the sidebar is collapsed.
            // Previously height shrank to 1px but the 16px of vertical
            // margin remained, leaving a ghost gap.
            margin: (!isMobile && collapsed) ? "0" : "12px 4px 4px",
            fontSize: "11px",
            fontWeight: 500,
            color: "var(--color-text-muted)",
            letterSpacing: "0.08em",
            textTransform: "uppercase",
            whiteSpace: "nowrap",
            overflow: "hidden",
            height: (!isMobile && collapsed) ? "0" : "auto",
            opacity: (!isMobile && collapsed) ? 0 : 1,
            transition: "opacity 150ms ease, height 150ms ease, margin 150ms ease",
          }}
        >
          Settings
        </div>

        {settingsNav.map((item) => (
          <SidebarNavItem
            key={item.to}
            item={item}
            collapsed={!isMobile && collapsed}
            onClick={isMobile ? onClose : undefined}
          />
        ))}
      </div>

      {/* Bottom area */}
      <div
        style={{
          borderTop: "1px solid var(--color-border-subtle)",
          padding: "8px",
          display: "flex",
          flexDirection: "column",
          gap: "4px",
        }}
      >
        <HealthDot collapsed={!isMobile && collapsed} />
        {!isMobile && !autoCollapsed && (
          <button
            onClick={onCollapse}
            style={{
              display: "flex",
              alignItems: "center",
              justifyContent: collapsed ? "center" : "flex-end",
              width: "100%",
              padding: "0 12px",
              height: "36px",
              background: "none",
              border: "none",
              cursor: "pointer",
              color: "var(--color-text-muted)",
              borderRadius: "6px",
              transition: "background 150ms ease, color 150ms ease",
            }}
            onMouseEnter={(e) => {
              (e.currentTarget as HTMLButtonElement).style.background = "var(--color-bg-elevated)";
              (e.currentTarget as HTMLButtonElement).style.color = "var(--color-text-secondary)";
            }}
            onMouseLeave={(e) => {
              (e.currentTarget as HTMLButtonElement).style.background = "none";
              (e.currentTarget as HTMLButtonElement).style.color = "var(--color-text-muted)";
            }}
            title={collapsed ? "Expand sidebar" : "Collapse sidebar"}
          >
            {collapsed ? (
              <ChevronRight size={16} strokeWidth={1.5} />
            ) : (
              <ChevronLeft size={16} strokeWidth={1.5} />
            )}
          </button>
        )}
      </div>
    </nav>
  );
}

export function Shell() {
  useWebSocket();

  useEffect(() => { applyTheme(); }, []);

  const [userCollapsed, setUserCollapsed] = useState(() => {
    return localStorage.getItem("sidebar-collapsed") === "true";
  });
  const [mobileOpen, setMobileOpen] = useState(false);
  const mode = useViewportMode();
  const isMobile = mode === "mobile";

  // In compact mode (768–1100px) the sidebar is force-collapsed
  // regardless of saved preference.
  const collapsed = mode === "compact" ? true : userCollapsed;

  useEffect(() => {
    if (!isMobile) setMobileOpen(false);
  }, [isMobile]);

  useEffect(() => {
    localStorage.setItem("sidebar-collapsed", String(userCollapsed));
  }, [userCollapsed]);

  const location = useLocation();
  useEffect(() => {
    window.scrollTo(0, 0);
    setMobileOpen(false);
  }, [location.pathname]);

  const desktopWidth = collapsed ? 60 : 240;

  return (
    <div style={{ display: "flex", minHeight: "100vh" }}>
      {isMobile && mobileOpen && (
        <div
          onClick={() => setMobileOpen(false)}
          style={{
            position: "fixed",
            inset: 0,
            background: "rgba(0,0,0,0.5)",
            zIndex: 49,
          }}
        />
      )}

      <div
        style={{
          transform: isMobile
            ? mobileOpen ? "translateX(0)" : "translateX(-100%)"
            : "none",
          transition: "transform 200ms ease",
        }}
      >
        <Sidebar
          collapsed={collapsed}
          onCollapse={() => setUserCollapsed((c) => !c)}
          onClose={() => setMobileOpen(false)}
          autoCollapsed={mode === "compact"}
          isMobile={isMobile}
        />
      </div>

      <main
        style={{
          flex: 1,
          marginLeft: isMobile ? 0 : desktopWidth,
          transition: "margin-left 200ms ease",
          minWidth: 0,
        }}
      >
        {isMobile && (
          <div
            style={{
              position: "sticky",
              top: 0,
              zIndex: 40,
              height: 52,
              background: "var(--color-bg-surface)",
              borderBottom: "1px solid var(--color-border-subtle)",
              display: "flex",
              alignItems: "center",
              padding: "0 16px",
              gap: 12,
            }}
          >
            <button
              onClick={() => setMobileOpen(true)}
              style={{
                background: "none",
                border: "none",
                cursor: "pointer",
                color: "var(--color-text-secondary)",
                display: "flex",
                alignItems: "center",
                padding: 4,
                borderRadius: 6,
              }}
            >
              <Menu size={20} />
            </button>
            <Link
              to="/"
              style={{ display: "flex", alignItems: "center", gap: 8, textDecoration: "none" }}
            >
              <div
                style={{
                  width: 24,
                  height: 24,
                  borderRadius: "6px",
                  background: "var(--color-accent)",
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "center",
                }}
              >
                <Network size={14} color="white" strokeWidth={2} />
              </div>
              <span
                style={{
                  fontSize: "15px",
                  fontWeight: 700,
                  color: "var(--color-text-primary)",
                  letterSpacing: "-0.01em",
                }}
              >
                Pulse
              </span>
            </Link>
          </div>
        )}

        <Outlet />
      </main>
    </div>
  );
}
