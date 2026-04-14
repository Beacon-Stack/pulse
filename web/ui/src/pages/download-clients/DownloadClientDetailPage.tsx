import { useState } from "react";
import { useParams, useNavigate } from "react-router-dom";
import {
  ArrowLeft, Trash2, Check, AlertCircle, Loader2,
} from "lucide-react";
import { useConfirm } from "@beacon-shared/ConfirmDialog";
import Pill from "@/components/Pill";
import { card, sectionHeader } from "@/lib/styles";
import { formatDate } from "@/lib/utils";
import {
  useDownloadClient,
  useUpdateDownloadClient,
  useDeleteDownloadClient,
  useTestDownloadClient,
} from "@/api/download-clients";
import type { TestResult } from "@/api/download-clients";

const CLIENT_KINDS = [
  { value: "qbittorrent", label: "qBittorrent", defaultPort: 8080 },
  { value: "deluge", label: "Deluge", defaultPort: 8112 },
  { value: "transmission", label: "Transmission", defaultPort: 9091 },
  { value: "sabnzbd", label: "SABnzbd", defaultPort: 8080 },
  { value: "nzbget", label: "NZBGet", defaultPort: 6789 },
];

export default function DownloadClientDetailPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { data: dc, isLoading } = useDownloadClient(id!);
  const updateDC = useUpdateDownloadClient();
  const deleteDC = useDeleteDownloadClient();
  const confirm = useConfirm();
  const testDC = useTestDownloadClient();

  const [testResult, setTestResult] = useState<TestResult | null>(null);
  const [editing, setEditing] = useState(false);
  const [name, setName] = useState("");
  const [kind, setKind] = useState("");
  const [host, setHost] = useState("");
  const [port, setPort] = useState("");
  const [useSSL, setUseSSL] = useState(false);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [category, setCategory] = useState("");
  const [directory, setDirectory] = useState("");
  const [enabled, setEnabled] = useState(true);
  const [priority, setPriority] = useState("1");

  // Populate fields when data loads
  if (dc && !editing && name === "") {
    setName(dc.name);
    setKind(dc.kind);
    setHost(dc.host);
    setPort(String(dc.port));
    setUseSSL(dc.use_ssl);
    setUsername(dc.username);
    setCategory(dc.category);
    setDirectory(dc.directory);
    setEnabled(dc.enabled);
    setPriority(String(dc.priority));
  }

  const handleTest = () => {
    setTestResult(null);
    testDC.mutate(
      { kind: kind || dc?.kind || "", host: host || dc?.host || "", port: parseInt(port) || dc?.port || 0, use_ssl: useSSL },
      { onSuccess: (r) => setTestResult(r) }
    );
  };

  const handleSave = () => {
    if (!dc) return;
    updateDC.mutate(
      {
        id: dc.id,
        name, kind, host,
        port: parseInt(port) || 0,
        use_ssl: useSSL,
        username, password: password || undefined,
        category, directory,
        enabled,
        priority: parseInt(priority) || 1,
      },
      { onSuccess: () => setEditing(false) }
    );
  };

  const handleDelete = async () => {
    if (!dc) return;
    if (
      !(await confirm({
        title: "Delete download client",
        message: `Delete "${dc.name}"? Services using it will lose their configuration.`,
        confirmLabel: "Delete",
      }))
    )
      return;
    deleteDC.mutate(dc.id, { onSuccess: () => navigate("/download-clients") });
  };

  const cancelEdit = () => {
    if (!dc) return;
    setEditing(false);
    setName(dc.name);
    setKind(dc.kind);
    setHost(dc.host);
    setPort(String(dc.port));
    setUseSSL(dc.use_ssl);
    setUsername(dc.username);
    setPassword("");
    setCategory(dc.category);
    setDirectory(dc.directory);
    setEnabled(dc.enabled);
    setPriority(String(dc.priority));
  };

  const inputStyle: React.CSSProperties = {
    width: "100%", padding: "8px 12px", borderRadius: 6,
    border: "1px solid var(--color-border-default)",
    background: "var(--color-bg-elevated)", color: "var(--color-text-primary)",
    fontSize: 13, outline: "none",
  };

  const labelStyle: React.CSSProperties = {
    display: "block", fontSize: 11, fontWeight: 600, color: "var(--color-text-muted)",
    textTransform: "uppercase", letterSpacing: "0.04em", marginBottom: 4,
  };

  if (isLoading) return <div style={{ padding: 24, color: "var(--color-text-muted)", fontSize: 13 }}>Loading...</div>;
  if (!dc) return (
    <div style={{ padding: 24 }}>
      <div style={{ color: "var(--color-danger)", fontSize: 14 }}>Download client not found</div>
      <button onClick={() => navigate("/download-clients")} style={{ marginTop: 12, background: "none", border: "none", color: "var(--color-accent)", cursor: "pointer", fontSize: 13 }}>Back</button>
    </div>
  );

  const kindLabel = CLIENT_KINDS.find((k) => k.value === dc.kind)?.label ?? dc.kind;

  return (
    <div style={{ padding: 24, maxWidth: 900 }}>
      {/* Header */}
      <div style={{ display: "flex", alignItems: "center", gap: 12, marginBottom: 20 }}>
        <button onClick={() => navigate("/download-clients")} style={{ display: "flex", alignItems: "center", justifyContent: "center", width: 32, height: 32, borderRadius: 6, border: "1px solid var(--color-border-default)", background: "transparent", cursor: "pointer", color: "var(--color-text-secondary)" }}>
          <ArrowLeft size={16} />
        </button>
        <div style={{ flex: 1 }}>
          <h1 style={{ margin: 0, fontSize: 20, fontWeight: 600, color: "var(--color-text-primary)" }}>{dc.name}</h1>
          <div style={{ display: "flex", gap: 6, marginTop: 4, alignItems: "center" }}>
            <span style={{ fontSize: 12, fontWeight: 600, padding: "2px 8px", borderRadius: 4, background: "var(--color-bg-subtle)", color: "var(--color-text-secondary)" }}>{kindLabel}</span>
            <span style={{ fontSize: 10, fontWeight: 600, padding: "2px 6px", borderRadius: 3, textTransform: "uppercase", color: dc.protocol === "torrent" ? "#3b9eff" : "#f59e0b", background: dc.protocol === "torrent" ? "color-mix(in srgb, #3b9eff 12%, transparent)" : "color-mix(in srgb, #f59e0b 12%, transparent)" }}>{dc.protocol}</span>
            <Pill ok={dc.enabled} labelTrue="Enabled" labelFalse="Disabled" />
          </div>
        </div>
        <div style={{ display: "flex", gap: 6 }}>
          {!editing ? (
            <button onClick={() => setEditing(true)} style={{ padding: "7px 14px", borderRadius: 6, border: "1px solid var(--color-border-default)", background: "transparent", color: "var(--color-text-secondary)", fontSize: 13, cursor: "pointer" }}>Edit</button>
          ) : (
            <>
              <button onClick={cancelEdit} style={{ padding: "7px 14px", borderRadius: 6, border: "1px solid var(--color-border-default)", background: "transparent", color: "var(--color-text-secondary)", fontSize: 13, cursor: "pointer" }}>Cancel</button>
              <button onClick={handleSave} disabled={updateDC.isPending} style={{ padding: "7px 14px", borderRadius: 6, border: "none", background: "var(--color-accent)", color: "var(--color-accent-fg)", fontSize: 13, fontWeight: 500, cursor: "pointer" }}>{updateDC.isPending ? "Saving..." : "Save"}</button>
            </>
          )}
          <button onClick={handleDelete} style={{ padding: "7px 14px", borderRadius: 6, border: "1px solid var(--color-border-default)", background: "transparent", color: "var(--color-danger)", fontSize: 13, cursor: "pointer", display: "flex", alignItems: "center", gap: 4 }}>
            <Trash2 size={13} /> Delete
          </button>
        </div>
      </div>

      {/* Connection */}
      <div style={{ ...card, marginBottom: 16 }}>
        <h3 style={sectionHeader}>Connection</h3>
        <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: "14px 24px" }}>
          <div>
            <label style={labelStyle}>Name</label>
            {editing ? <input style={inputStyle} value={name} onChange={(e) => setName(e.target.value)} /> : <div style={{ fontSize: 14, color: "var(--color-text-primary)", fontWeight: 500 }}>{dc.name}</div>}
          </div>
          <div>
            <label style={labelStyle}>Type</label>
            {editing ? (
              <select style={inputStyle} value={kind} onChange={(e) => setKind(e.target.value)}>
                {CLIENT_KINDS.map((k) => <option key={k.value} value={k.value}>{k.label}</option>)}
              </select>
            ) : <div style={{ fontSize: 14, color: "var(--color-text-primary)" }}>{kindLabel}</div>}
          </div>
          <div>
            <label style={labelStyle}>Host</label>
            {editing ? <input style={inputStyle} value={host} onChange={(e) => setHost(e.target.value)} /> : <div style={{ fontSize: 13, color: "var(--color-text-secondary)", fontFamily: "var(--font-family-mono)" }}>{dc.host}</div>}
          </div>
          <div>
            <label style={labelStyle}>Port</label>
            {editing ? <input style={inputStyle} type="number" value={port} onChange={(e) => setPort(e.target.value)} /> : <div style={{ fontSize: 13, color: "var(--color-text-secondary)", fontFamily: "var(--font-family-mono)" }}>{dc.port}</div>}
          </div>
          <div>
            <label style={labelStyle}>SSL</label>
            {editing ? (
              <label style={{ display: "flex", alignItems: "center", gap: 8, cursor: "pointer", fontSize: 13, color: "var(--color-text-secondary)" }}>
                <input type="checkbox" checked={useSSL} onChange={(e) => setUseSSL(e.target.checked)} /> {useSSL ? "Yes" : "No"}
              </label>
            ) : <div style={{ fontSize: 13, color: "var(--color-text-secondary)" }}>{dc.use_ssl ? "Yes" : "No"}</div>}
          </div>
          <div>
            <label style={labelStyle}>Username</label>
            {editing ? <input style={inputStyle} value={username} onChange={(e) => setUsername(e.target.value)} /> : <div style={{ fontSize: 13, color: "var(--color-text-secondary)" }}>{dc.username || "—"}</div>}
          </div>
          {editing && (
            <div>
              <label style={labelStyle}>Password</label>
              <input style={inputStyle} type="password" value={password} onChange={(e) => setPassword(e.target.value)} placeholder="Leave blank to keep existing" />
            </div>
          )}
          <div>
            <label style={labelStyle}>Category</label>
            {editing ? <input style={inputStyle} value={category} onChange={(e) => setCategory(e.target.value)} /> : <div style={{ fontSize: 13, color: "var(--color-text-secondary)" }}>{dc.category || "—"}</div>}
          </div>
          <div>
            <label style={labelStyle}>Directory</label>
            {editing ? <input style={inputStyle} value={directory} onChange={(e) => setDirectory(e.target.value)} /> : <div style={{ fontSize: 13, color: "var(--color-text-secondary)", fontFamily: "var(--font-family-mono)" }}>{dc.directory || "—"}</div>}
          </div>
          <div>
            <label style={labelStyle}>Priority</label>
            {editing ? <input style={inputStyle} type="number" value={priority} onChange={(e) => setPriority(e.target.value)} /> : <div style={{ fontSize: 13, color: "var(--color-text-secondary)" }}>{dc.priority}</div>}
          </div>
          <div>
            <label style={labelStyle}>Enabled</label>
            {editing ? (
              <label style={{ display: "flex", alignItems: "center", gap: 8, cursor: "pointer", fontSize: 13, color: "var(--color-text-secondary)" }}>
                <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} /> {enabled ? "Enabled" : "Disabled"}
              </label>
            ) : <Pill ok={dc.enabled} labelTrue="Enabled" labelFalse="Disabled" />}
          </div>
          <div>
            <label style={labelStyle}>Created</label>
            <div style={{ fontSize: 13, color: "var(--color-text-secondary)" }}>{formatDate(dc.created_at, true)}</div>
          </div>
          <div>
            <label style={labelStyle}>Updated</label>
            <div style={{ fontSize: 13, color: "var(--color-text-secondary)" }}>{formatDate(dc.updated_at, true)}</div>
          </div>
        </div>
      </div>

      {/* Test */}
      <div style={card}>
        <h3 style={sectionHeader}>Connection Test</h3>
        <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
          <button onClick={handleTest} disabled={testDC.isPending} style={{ padding: "7px 16px", borderRadius: 6, border: "1px solid var(--color-border-default)", background: "var(--color-bg-elevated)", color: "var(--color-text-secondary)", fontSize: 13, cursor: "pointer", display: "flex", alignItems: "center", gap: 6 }}>
            {testDC.isPending ? <><Loader2 size={14} style={{ animation: "spin 1s linear infinite" }} /> Testing...</> : "Test Connection"}
          </button>
          {testResult?.success && (
            <span style={{ display: "flex", alignItems: "center", gap: 4, fontSize: 13, color: "var(--color-success)" }}>
              <Check size={14} /> {testResult.message} ({testResult.duration})
            </span>
          )}
          {testResult && !testResult.success && (
            <span style={{ display: "flex", alignItems: "center", gap: 4, fontSize: 13, color: "var(--color-danger)" }}>
              <AlertCircle size={14} /> {testResult.message}
            </span>
          )}
        </div>
      </div>
    </div>
  );
}
