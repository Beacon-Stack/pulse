import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { Plus, ChevronRight } from "lucide-react";
import PageHeader from "@/components/PageHeader";
import StatusBadge from "@/components/StatusBadge";
import Modal from "@/components/Modal";
import { useServices, useRegisterService } from "@/api/services";
import { card } from "@/lib/styles";
import { timeAgo } from "@/lib/utils";

const serviceTypes = [
  "download-client",
  "media-manager",
  "indexer",
  "notification",
  "metadata",
  "automation",
];

function RegisterModal({ onClose }: { onClose: () => void }) {
  const register = useRegisterService();
  const [name, setName] = useState("");
  const [type, setType] = useState(serviceTypes[0]);
  const [apiUrl, setApiUrl] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [healthUrl, setHealthUrl] = useState("");
  const [version, setVersion] = useState("");
  const [capabilities, setCapabilities] = useState("");

  const inputStyle: React.CSSProperties = {
    width: "100%", padding: "8px 12px", borderRadius: 6,
    border: "1px solid var(--color-border-default)",
    background: "var(--color-bg-elevated)", color: "var(--color-text-primary)",
    fontSize: 13, outline: "none",
  };

  const labelStyle: React.CSSProperties = {
    display: "block", fontSize: 12, fontWeight: 500,
    color: "var(--color-text-secondary)", marginBottom: 4,
  };

  return (
    <Modal onClose={onClose} width={480}>
      <div style={{ padding: 20 }}>
        <h2 style={{ margin: "0 0 16px", fontSize: 16, fontWeight: 600, color: "var(--color-text-primary)" }}>Register Service</h2>
        <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
          <div><label style={labelStyle}>Name</label><input style={inputStyle} value={name} onChange={(e) => setName(e.target.value)} placeholder="qbittorrent" /></div>
          <div><label style={labelStyle}>Type</label><select style={inputStyle} value={type} onChange={(e) => setType(e.target.value)}>{serviceTypes.map((t) => <option key={t} value={t}>{t}</option>)}</select></div>
          <div><label style={labelStyle}>API URL</label><input style={inputStyle} value={apiUrl} onChange={(e) => setApiUrl(e.target.value)} placeholder="http://qbit:8080" /></div>
          <div><label style={labelStyle}>API Key</label><input style={inputStyle} value={apiKey} onChange={(e) => setApiKey(e.target.value)} placeholder="Optional" /></div>
          <div><label style={labelStyle}>Health URL</label><input style={inputStyle} value={healthUrl} onChange={(e) => setHealthUrl(e.target.value)} placeholder="http://qbit:8080/health" /></div>
          <div><label style={labelStyle}>Version</label><input style={inputStyle} value={version} onChange={(e) => setVersion(e.target.value)} placeholder="4.6.3" /></div>
          <div><label style={labelStyle}>Capabilities (comma-separated)</label><input style={inputStyle} value={capabilities} onChange={(e) => setCapabilities(e.target.value)} placeholder="supports_torrent, content:movies" /></div>
        </div>
        <div style={{ display: "flex", justifyContent: "flex-end", gap: 8, marginTop: 20 }}>
          <button onClick={onClose} style={{ padding: "7px 16px", borderRadius: 6, border: "1px solid var(--color-border-default)", background: "transparent", color: "var(--color-text-secondary)", fontSize: 13, cursor: "pointer" }}>Cancel</button>
          <button
            onClick={() => register.mutate({ name, type, api_url: apiUrl, api_key: apiKey || undefined, health_url: healthUrl || undefined, version: version || undefined, capabilities: capabilities ? capabilities.split(",").map((c) => c.trim()).filter(Boolean) : undefined }, { onSuccess: onClose })}
            disabled={!name || !apiUrl || register.isPending}
            style={{ padding: "7px 16px", borderRadius: 6, border: "none", background: "var(--color-accent)", color: "var(--color-accent-fg)", fontSize: 13, fontWeight: 500, cursor: "pointer", opacity: !name || !apiUrl ? 0.5 : 1 }}
          >{register.isPending ? "Registering..." : "Register"}</button>
        </div>
      </div>
    </Modal>
  );
}

export default function ServicesPage() {
  const navigate = useNavigate();
  const { data: services, isLoading } = useServices();
  const [showRegister, setShowRegister] = useState(false);

  return (
    <div style={{ padding: 24, maxWidth: 1200 }}>
      <PageHeader
        title="Services"
        description="Registered ecosystem services. Click a service to see details, indexers, and health."
        action={
          <button
            onClick={() => setShowRegister(true)}
            style={{ display: "flex", alignItems: "center", gap: 6, padding: "7px 14px", borderRadius: 6, border: "none", background: "var(--color-accent)", color: "var(--color-accent-fg)", fontSize: 13, fontWeight: 500, cursor: "pointer" }}
          >
            <Plus size={15} /> Register
          </button>
        }
      />

      {isLoading ? (
        <div style={{ color: "var(--color-text-muted)", fontSize: 13 }}>Loading...</div>
      ) : !services?.length ? (
        <div style={{ ...card, color: "var(--color-text-muted)", fontSize: 13, textAlign: "center", padding: 40 }}>
          No services registered. Services will appear here once they connect to Pulse.
        </div>
      ) : (
        <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
          {services.map((svc) => (
            <button
              key={svc.id}
              onClick={() => navigate(`/services/${svc.id}`)}
              style={{
                display: "flex", alignItems: "center", gap: 12,
                padding: "12px 16px", borderRadius: 8,
                border: "1px solid var(--color-border-subtle)",
                background: "var(--color-bg-surface)",
                cursor: "pointer", textAlign: "left", width: "100%",
                transition: "border-color 120ms ease",
              }}
              onMouseEnter={(e) => { e.currentTarget.style.borderColor = "var(--color-border-default)"; }}
              onMouseLeave={(e) => { e.currentTarget.style.borderColor = "var(--color-border-subtle)"; }}
            >
              <StatusBadge status={svc.status} />
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                  <span style={{ fontSize: 15, fontWeight: 600, color: "var(--color-text-primary)" }}>{svc.name}</span>
                  <span style={{ fontSize: 11, color: "var(--color-text-muted)", background: "var(--color-bg-subtle)", padding: "2px 8px", borderRadius: 4 }}>{svc.type}</span>
                  {svc.version && (
                    <span style={{ fontSize: 11, color: "var(--color-text-muted)", fontFamily: "var(--font-family-mono)" }}>{svc.version}</span>
                  )}
                </div>
                <div style={{ fontSize: 12, color: "var(--color-text-muted)", fontFamily: "var(--font-family-mono)", marginTop: 3, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                  {svc.api_url}
                </div>
              </div>
              <span style={{ fontSize: 12, color: "var(--color-text-muted)", flexShrink: 0 }}>
                {svc.last_seen ? timeAgo(svc.last_seen) : ""}
              </span>
              <ChevronRight size={16} style={{ color: "var(--color-text-muted)", flexShrink: 0 }} />
            </button>
          ))}
        </div>
      )}

      {showRegister && <RegisterModal onClose={() => setShowRegister(false)} />}
    </div>
  );
}
