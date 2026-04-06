import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { Plus, Check, AlertCircle, Loader2, ChevronRight } from "lucide-react";
import PageHeader from "@/components/PageHeader";
import Pill from "@/components/Pill";
import Modal from "@/components/Modal";
import { useDownloadClients, useCreateDownloadClient, useTestDownloadClient } from "@/api/download-clients";
import { card } from "@/lib/styles";
import type { TestResult } from "@/api/download-clients";

const CLIENT_KINDS = [
  { value: "qbittorrent", label: "qBittorrent", protocol: "torrent", defaultPort: 8080 },
  { value: "deluge", label: "Deluge", protocol: "torrent", defaultPort: 8112 },
  { value: "transmission", label: "Transmission", protocol: "torrent", defaultPort: 9091 },
  { value: "sabnzbd", label: "SABnzbd", protocol: "usenet", defaultPort: 8080 },
  { value: "nzbget", label: "NZBGet", protocol: "usenet", defaultPort: 6789 },
] as const;

const kindColors: Record<string, string> = {
  qbittorrent: "#3b9eff",
  deluge: "#4fc3f7",
  transmission: "#f44336",
  sabnzbd: "#f59e0b",
  nzbget: "#34d399",
};

function AddModal({ onClose }: { onClose: () => void }) {
  const create = useCreateDownloadClient();
  const testDC = useTestDownloadClient();
  const [kind, setKind] = useState("qbittorrent");
  const [name, setName] = useState("");
  const [host, setHost] = useState("localhost");
  const [port, setPort] = useState("8080");
  const [useSSL, setUseSSL] = useState(false);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [category, setCategory] = useState("");
  const [testResult, setTestResult] = useState<TestResult | null>(null);

  const selectedKind = CLIENT_KINDS.find((k) => k.value === kind);

  const handleKindChange = (newKind: string) => {
    setKind(newKind);
    const k = CLIENT_KINDS.find((c) => c.value === newKind);
    if (k) {
      setPort(String(k.defaultPort));
      if (!name) setName(k.label);
    }
  };

  const handleTest = () => {
    setTestResult(null);
    testDC.mutate(
      { kind, host, port: parseInt(port) || 0, use_ssl: useSSL },
      { onSuccess: (r) => setTestResult(r) }
    );
  };

  const handleSave = () => {
    create.mutate(
      {
        name: name || selectedKind?.label || kind,
        kind,
        protocol: selectedKind?.protocol || "torrent",
        host,
        port: parseInt(port) || 0,
        use_ssl: useSSL,
        username,
        password,
        category,
      },
      { onSuccess: onClose }
    );
  };

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
    <Modal onClose={onClose} width={500}>
      <div style={{ padding: 20 }}>
        <h2 style={{ margin: "0 0 16px", fontSize: 16, fontWeight: 600, color: "var(--color-text-primary)" }}>
          Add Download Client
        </h2>

        <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
          {/* Kind selector */}
          <div>
            <label style={labelStyle}>Type</label>
            <div style={{ display: "flex", gap: 6, flexWrap: "wrap" }}>
              {CLIENT_KINDS.map((k) => (
                <button
                  key={k.value}
                  onClick={() => handleKindChange(k.value)}
                  style={{
                    padding: "6px 14px", borderRadius: 6, fontSize: 13, fontWeight: 500, cursor: "pointer",
                    border: kind === k.value ? "1px solid var(--color-accent)" : "1px solid var(--color-border-default)",
                    background: kind === k.value ? "var(--color-accent-muted)" : "transparent",
                    color: kind === k.value ? "var(--color-accent)" : "var(--color-text-secondary)",
                  }}
                >
                  {k.label}
                </button>
              ))}
            </div>
          </div>

          <div><label style={labelStyle}>Name</label><input style={inputStyle} value={name} onChange={(e) => setName(e.target.value)} placeholder={selectedKind?.label} /></div>

          <div style={{ display: "grid", gridTemplateColumns: "1fr 100px", gap: 12 }}>
            <div><label style={labelStyle}>Host</label><input style={inputStyle} value={host} onChange={(e) => setHost(e.target.value)} placeholder="localhost" /></div>
            <div><label style={labelStyle}>Port</label><input style={inputStyle} type="number" value={port} onChange={(e) => setPort(e.target.value)} /></div>
          </div>

          <label style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 13, color: "var(--color-text-secondary)", cursor: "pointer" }}>
            <input type="checkbox" checked={useSSL} onChange={(e) => setUseSSL(e.target.checked)} /> Use SSL
          </label>

          <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 12 }}>
            <div><label style={labelStyle}>Username</label><input style={inputStyle} value={username} onChange={(e) => setUsername(e.target.value)} placeholder="Optional" /></div>
            <div><label style={labelStyle}>Password</label><input style={inputStyle} type="password" value={password} onChange={(e) => setPassword(e.target.value)} placeholder="Optional" /></div>
          </div>

          <div><label style={labelStyle}>Category / Label</label><input style={inputStyle} value={category} onChange={(e) => setCategory(e.target.value)} placeholder="e.g., movies, tv" /></div>
        </div>

        {/* Test + Save */}
        <div style={{ display: "flex", alignItems: "center", gap: 8, marginTop: 20 }}>
          <button onClick={handleTest} disabled={testDC.isPending} style={{ padding: "7px 14px", borderRadius: 6, border: "1px solid var(--color-border-default)", background: "transparent", color: "var(--color-text-secondary)", fontSize: 13, cursor: "pointer", display: "flex", alignItems: "center", gap: 5 }}>
            {testDC.isPending ? <><Loader2 size={13} style={{ animation: "spin 1s linear infinite" }} /> Testing...</> : "Test"}
          </button>
          {testResult && (
            <span style={{ fontSize: 12, color: testResult.success ? "var(--color-success)" : "var(--color-danger)", display: "flex", alignItems: "center", gap: 4 }}>
              {testResult.success ? <Check size={13} /> : <AlertCircle size={13} />}
              {testResult.message}
            </span>
          )}
          <div style={{ flex: 1 }} />
          <button onClick={onClose} style={{ padding: "7px 16px", borderRadius: 6, border: "1px solid var(--color-border-default)", background: "transparent", color: "var(--color-text-secondary)", fontSize: 13, cursor: "pointer" }}>Cancel</button>
          <button onClick={handleSave} disabled={!host || create.isPending} style={{ padding: "7px 16px", borderRadius: 6, border: "none", background: "var(--color-accent)", color: "var(--color-accent-fg)", fontSize: 13, fontWeight: 500, cursor: "pointer", opacity: !host ? 0.5 : 1 }}>
            {create.isPending ? "Saving..." : "Save"}
          </button>
        </div>
      </div>
    </Modal>
  );
}

export default function DownloadClientsPage() {
  const navigate = useNavigate();
  const { data: clients, isLoading } = useDownloadClients();
  const [showAdd, setShowAdd] = useState(false);

  return (
    <div style={{ padding: 24, maxWidth: 1200 }}>
      <PageHeader
        title="Download Clients"
        description="Centrally managed download clients. Auto-pushed to all connected media managers."
        action={
          <button
            onClick={() => setShowAdd(true)}
            style={{ display: "flex", alignItems: "center", gap: 6, padding: "7px 14px", borderRadius: 6, border: "none", background: "var(--color-accent)", color: "var(--color-accent-fg)", fontSize: 13, fontWeight: 500, cursor: "pointer" }}
          >
            <Plus size={15} /> Add Download Client
          </button>
        }
      />

      {isLoading ? (
        <div style={{ color: "var(--color-text-muted)", fontSize: 13 }}>Loading...</div>
      ) : !clients?.length ? (
        <div style={{ ...card, color: "var(--color-text-muted)", fontSize: 13, textAlign: "center", padding: 40 }}>
          No download clients configured. Add one to manage it centrally across all services.
        </div>
      ) : (
        <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
          {clients.map((dc) => (
            <div
              key={dc.id}
              onClick={() => navigate(`/download-clients/${dc.id}`)}
              style={{
                display: "flex", alignItems: "center", gap: 12,
                padding: "12px 16px", borderRadius: 8,
                border: "1px solid var(--color-border-subtle)",
                background: "var(--color-bg-surface)",
                cursor: "pointer",
                transition: "border-color 120ms ease",
              }}
              onMouseEnter={(e) => { e.currentTarget.style.borderColor = "var(--color-border-default)"; }}
              onMouseLeave={(e) => { e.currentTarget.style.borderColor = "var(--color-border-subtle)"; }}
            >
              <div style={{
                width: 36, height: 36, borderRadius: 8, flexShrink: 0,
                background: `color-mix(in srgb, ${kindColors[dc.kind] ?? "var(--color-accent)"} 12%, transparent)`,
                display: "flex", alignItems: "center", justifyContent: "center",
                fontSize: 11, fontWeight: 700, color: kindColors[dc.kind] ?? "var(--color-accent)",
                textTransform: "uppercase",
              }}>
                {dc.kind.slice(0, 3)}
              </div>

              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                  <span style={{ fontSize: 15, fontWeight: 600, color: "var(--color-text-primary)" }}>{dc.name}</span>
                  <span style={{
                    fontSize: 10, fontWeight: 600, padding: "2px 6px", borderRadius: 3,
                    textTransform: "uppercase", letterSpacing: "0.04em",
                    color: dc.protocol === "torrent" ? "#3b9eff" : "#f59e0b",
                    background: dc.protocol === "torrent"
                      ? "color-mix(in srgb, #3b9eff 12%, transparent)"
                      : "color-mix(in srgb, #f59e0b 12%, transparent)",
                  }}>
                    {dc.protocol}
                  </span>
                  <Pill ok={dc.enabled} labelTrue="Enabled" labelFalse="Disabled" />
                  {dc.category && (
                    <span style={{ fontSize: 11, color: "var(--color-text-muted)", background: "var(--color-bg-subtle)", padding: "1px 6px", borderRadius: 3 }}>
                      {dc.category}
                    </span>
                  )}
                </div>
                <div style={{ fontSize: 12, color: "var(--color-text-muted)", fontFamily: "var(--font-family-mono)", marginTop: 3 }}>
                  {dc.use_ssl ? "https" : "http"}://{dc.host}:{dc.port}
                </div>
              </div>

              <ChevronRight size={16} style={{ color: "var(--color-text-muted)", flexShrink: 0 }} />
            </div>
          ))}
        </div>
      )}

      {showAdd && <AddModal onClose={() => setShowAdd(false)} />}
    </div>
  );
}
