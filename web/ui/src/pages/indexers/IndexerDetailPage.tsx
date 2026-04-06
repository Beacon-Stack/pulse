import { useState } from "react";
import { useParams, useNavigate } from "react-router-dom";
import {
  ArrowLeft,
  Trash2,
  Check,
  AlertCircle,
  Loader2,
  Server,
  X,
} from "lucide-react";
import Pill from "@/components/Pill";
import { card, sectionHeader } from "@/lib/styles";
import { formatDate } from "@/lib/utils";
import {
  useIndexer,
  useUpdateIndexer,
  useDeleteIndexer,
  useIndexerAssignments,
  useUnassignIndexer,
} from "@/api/indexers";
import { useServices } from "@/api/services";
import { useCatalog } from "@/api/catalog";

const categoryColors: Record<string, string> = {
  Movies: "#3b9eff",
  TV: "#34d399",
  Audio: "#f59e0b",
  Books: "#a78bfa",
  XXX: "#f87171",
  Other: "#6b7280",
};

export default function IndexerDetailPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { data: indexer, isLoading } = useIndexer(id!);
  const { data: assignments } = useIndexerAssignments(id!);
  const { data: services } = useServices();
  const updateIndexer = useUpdateIndexer();
  const deleteIndexer = useDeleteIndexer();
  const unassign = useUnassignIndexer();
  const { data: catalogData } = useCatalog();

  // Look up catalog entry to get categories, description, language, privacy
  const catalogEntry = catalogData?.entries.find(
    (e) => e.name.toLowerCase() === indexer?.name.toLowerCase()
  );

  const [testStatus, setTestStatus] = useState<"idle" | "testing" | "success" | "error">("idle");
  const [testMessage, setTestMessage] = useState("");

  // Editable fields
  const [editing, setEditing] = useState(false);
  const [name, setName] = useState("");
  const [url, setUrl] = useState("");
  const [kind, setKind] = useState("");
  const [priority, setPriority] = useState("");
  const [enabled, setEnabled] = useState(true);

  // Populate fields when data loads
  if (indexer && !editing && name === "") {
    setName(indexer.name);
    setUrl(indexer.url);
    setKind(indexer.kind);
    setPriority(String(indexer.priority));
    setEnabled(indexer.enabled);
  }

  const handleTest = async () => {
    if (!indexer) return;
    setTestStatus("testing");
    setTestMessage("");
    try {
      // Use the search-based test which validates the indexer actually works
      const res = await fetch(`/api/v1/indexers/${indexer.id}/test-search`, { method: "POST" });
      const data = (await res.json()) as { success: boolean; message: string; duration: string; results: number };
      setTestStatus(data.success ? "success" : "error");
      setTestMessage(data.message + (data.duration ? ` (${data.duration})` : ""));
    } catch {
      setTestStatus("error");
      setTestMessage("Request failed");
    }
  };

  const handleSave = () => {
    if (!indexer) return;
    updateIndexer.mutate(
      {
        id: indexer.id,
        name,
        kind,
        url,
        enabled,
        priority: parseInt(priority) || 25,
      },
      { onSuccess: () => setEditing(false) }
    );
  };

  const handleDelete = () => {
    if (!indexer) return;
    if (!confirm(`Delete indexer "${indexer.name}"? This cannot be undone.`)) return;
    deleteIndexer.mutate(indexer.id, { onSuccess: () => navigate("/indexers") });
  };

  // Look up service names for assignments
  const serviceMap = new Map(services?.map((s) => [s.id, s]) ?? []);

  const inputStyle: React.CSSProperties = {
    width: "100%",
    padding: "8px 12px",
    borderRadius: 6,
    border: "1px solid var(--color-border-default)",
    background: "var(--color-bg-elevated)",
    color: "var(--color-text-primary)",
    fontSize: 13,
    outline: "none",
  };

  const labelStyle: React.CSSProperties = {
    display: "block",
    fontSize: 12,
    fontWeight: 500,
    color: "var(--color-text-muted)",
    marginBottom: 4,
    textTransform: "uppercase",
    letterSpacing: "0.04em",
  };

  if (isLoading) {
    return (
      <div style={{ padding: 24, color: "var(--color-text-muted)", fontSize: 13 }}>Loading...</div>
    );
  }

  if (!indexer) {
    return (
      <div style={{ padding: 24 }}>
        <div style={{ color: "var(--color-danger)", fontSize: 14 }}>Indexer not found</div>
        <button onClick={() => navigate("/indexers")} style={{ marginTop: 12, background: "none", border: "none", color: "var(--color-accent)", cursor: "pointer", fontSize: 13 }}>
          Back to Indexers
        </button>
      </div>
    );
  }

  return (
    <div style={{ padding: 24, maxWidth: 900 }}>
      {/* Back + title */}
      <div style={{ display: "flex", alignItems: "center", gap: 12, marginBottom: 20 }}>
        <button
          onClick={() => navigate("/indexers")}
          style={{
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            width: 32,
            height: 32,
            borderRadius: 6,
            border: "1px solid var(--color-border-default)",
            background: "transparent",
            cursor: "pointer",
            color: "var(--color-text-secondary)",
          }}
        >
          <ArrowLeft size={16} />
        </button>
        <div style={{ flex: 1 }}>
          <h1 style={{ margin: 0, fontSize: 20, fontWeight: 600, color: "var(--color-text-primary)" }}>
            {indexer.name}
          </h1>
          <div style={{ display: "flex", gap: 6, marginTop: 4 }}>
            <span style={{ fontSize: 11, padding: "2px 8px", borderRadius: 4, background: indexer.kind === "torznab" ? "color-mix(in srgb, #3b9eff 12%, transparent)" : "color-mix(in srgb, #f59e0b 12%, transparent)", color: indexer.kind === "torznab" ? "#3b9eff" : "#f59e0b", fontWeight: 600, textTransform: "uppercase", letterSpacing: "0.04em" }}>
              {indexer.kind}
            </span>
            <Pill ok={indexer.enabled} labelTrue="Enabled" labelFalse="Disabled" />
          </div>
        </div>

        <div style={{ display: "flex", gap: 6 }}>
          {!editing ? (
            <button
              onClick={() => setEditing(true)}
              style={{ padding: "7px 14px", borderRadius: 6, border: "1px solid var(--color-border-default)", background: "transparent", color: "var(--color-text-secondary)", fontSize: 13, cursor: "pointer" }}
            >
              Edit
            </button>
          ) : (
            <>
              <button
                onClick={() => { setEditing(false); setName(indexer.name); setUrl(indexer.url); setKind(indexer.kind); setPriority(String(indexer.priority)); setEnabled(indexer.enabled); }}
                style={{ padding: "7px 14px", borderRadius: 6, border: "1px solid var(--color-border-default)", background: "transparent", color: "var(--color-text-secondary)", fontSize: 13, cursor: "pointer" }}
              >
                Cancel
              </button>
              <button
                onClick={handleSave}
                disabled={updateIndexer.isPending}
                style={{ padding: "7px 14px", borderRadius: 6, border: "none", background: "var(--color-accent)", color: "var(--color-accent-fg)", fontSize: 13, fontWeight: 500, cursor: "pointer" }}
              >
                {updateIndexer.isPending ? "Saving..." : "Save"}
              </button>
            </>
          )}
          <button
            onClick={handleDelete}
            style={{ padding: "7px 14px", borderRadius: 6, border: "1px solid var(--color-border-default)", background: "transparent", color: "var(--color-danger)", fontSize: 13, cursor: "pointer", display: "flex", alignItems: "center", gap: 4 }}
          >
            <Trash2 size={13} /> Delete
          </button>
        </div>
      </div>

      {/* Catalog info */}
      {catalogEntry && (
        <div style={{ ...card, marginBottom: 16 }}>
          {catalogEntry.description && (
            <p style={{ margin: "0 0 12px", fontSize: 13, color: "var(--color-text-secondary)", lineHeight: 1.5 }}>
              {catalogEntry.description}
            </p>
          )}
          <div style={{ display: "flex", flexWrap: "wrap", gap: 6, alignItems: "center" }}>
            {catalogEntry.categories.map((cat) => (
              <span
                key={cat}
                style={{
                  fontSize: 12,
                  fontWeight: 500,
                  padding: "3px 10px",
                  borderRadius: 4,
                  color: categoryColors[cat] ?? "#6b7280",
                  background: `color-mix(in srgb, ${categoryColors[cat] ?? "#6b7280"} 12%, transparent)`,
                }}
              >
                {cat}
              </span>
            ))}
            {catalogEntry.privacy && (
              <span style={{
                fontSize: 11,
                fontWeight: 500,
                padding: "3px 8px",
                borderRadius: 4,
                color: catalogEntry.privacy === "public" ? "var(--color-success)" : catalogEntry.privacy === "private" ? "var(--color-danger)" : "var(--color-warning)",
                background: catalogEntry.privacy === "public"
                  ? "color-mix(in srgb, var(--color-success) 10%, transparent)"
                  : catalogEntry.privacy === "private"
                  ? "color-mix(in srgb, var(--color-danger) 10%, transparent)"
                  : "color-mix(in srgb, var(--color-warning) 10%, transparent)",
              }}>
                {catalogEntry.privacy}
              </span>
            )}
            {catalogEntry.language && catalogEntry.language !== "en-US" && (
              <span style={{ fontSize: 11, padding: "3px 8px", borderRadius: 4, background: "var(--color-bg-subtle)", color: "var(--color-text-muted)" }}>
                {catalogEntry.language}
              </span>
            )}
          </div>
        </div>
      )}

      {/* Details card */}
      <div style={{ ...card, marginBottom: 16 }}>
        <h3 style={sectionHeader}>Configuration</h3>
        <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: "16px 24px" }}>
          <div>
            <label style={labelStyle}>Name</label>
            {editing ? (
              <input style={inputStyle} value={name} onChange={(e) => setName(e.target.value)} />
            ) : (
              <div style={{ fontSize: 14, color: "var(--color-text-primary)", fontWeight: 500 }}>{indexer.name}</div>
            )}
          </div>
          <div>
            <label style={labelStyle}>Type</label>
            {editing ? (
              <select style={inputStyle} value={kind} onChange={(e) => setKind(e.target.value)}>
                <option value="torznab">Torznab</option>
                <option value="newznab">Newznab</option>
              </select>
            ) : (
              <div style={{ fontSize: 14, color: "var(--color-text-primary)" }}>{indexer.kind}</div>
            )}
          </div>
          <div style={{ gridColumn: "1 / -1" }}>
            <label style={labelStyle}>URL</label>
            {editing ? (
              <input style={inputStyle} value={url} onChange={(e) => setUrl(e.target.value)} />
            ) : (
              <div style={{ fontSize: 13, color: "var(--color-text-secondary)", fontFamily: "var(--font-family-mono)", wordBreak: "break-all" }}>{indexer.url}</div>
            )}
          </div>
          <div>
            <label style={labelStyle}>Priority</label>
            {editing ? (
              <input style={inputStyle} type="number" value={priority} onChange={(e) => setPriority(e.target.value)} />
            ) : (
              <div style={{ fontSize: 14, color: "var(--color-text-primary)", fontFamily: "var(--font-family-mono)" }}>{indexer.priority}</div>
            )}
          </div>
          <div>
            <label style={labelStyle}>Enabled</label>
            {editing ? (
              <label style={{ display: "flex", alignItems: "center", gap: 8, cursor: "pointer", fontSize: 13, color: "var(--color-text-secondary)" }}>
                <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />
                {enabled ? "Enabled" : "Disabled"}
              </label>
            ) : (
              <Pill ok={indexer.enabled} labelTrue="Enabled" labelFalse="Disabled" />
            )}
          </div>
          <div>
            <label style={labelStyle}>Created</label>
            <div style={{ fontSize: 13, color: "var(--color-text-secondary)" }}>{formatDate(indexer.created_at, true)}</div>
          </div>
          <div>
            <label style={labelStyle}>Updated</label>
            <div style={{ fontSize: 13, color: "var(--color-text-secondary)" }}>{formatDate(indexer.updated_at, true)}</div>
          </div>
        </div>
      </div>

      {/* Test card */}
      <div style={{ ...card, marginBottom: 16 }}>
        <h3 style={sectionHeader}>Connection Test</h3>
        <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
          <button
            onClick={handleTest}
            disabled={testStatus === "testing"}
            style={{
              padding: "7px 16px",
              borderRadius: 6,
              border: "1px solid var(--color-border-default)",
              background: "var(--color-bg-elevated)",
              color: "var(--color-text-secondary)",
              fontSize: 13,
              cursor: "pointer",
              display: "flex",
              alignItems: "center",
              gap: 6,
            }}
          >
            {testStatus === "testing" ? (
              <><Loader2 size={14} style={{ animation: "spin 1s linear infinite" }} /> Testing...</>
            ) : (
              "Test Connection"
            )}
          </button>
          {testStatus === "success" && (
            <span style={{ display: "flex", alignItems: "center", gap: 4, fontSize: 13, color: "var(--color-success)" }}>
              <Check size={14} /> {testMessage}
            </span>
          )}
          {testStatus === "error" && (
            <span style={{ display: "flex", alignItems: "center", gap: 4, fontSize: 13, color: "var(--color-danger)" }}>
              <AlertCircle size={14} /> {testMessage}
            </span>
          )}
        </div>
      </div>

      {/* Assigned services card */}
      <div style={card}>
        <h3 style={sectionHeader}>Assigned Services</h3>
        {!assignments?.length ? (
          <div style={{ fontSize: 13, color: "var(--color-text-muted)" }}>
            Not assigned to any services. Indexers are auto-assigned based on category matching when added.
          </div>
        ) : (
          <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
            {assignments.map((a) => {
              const svc = serviceMap.get(a.serviceId);
              return (
                <div
                  key={a.id}
                  style={{
                    display: "flex",
                    alignItems: "center",
                    gap: 10,
                    padding: "8px 12px",
                    borderRadius: 6,
                    border: "1px solid var(--color-border-subtle)",
                    background: "var(--color-bg-elevated)",
                  }}
                >
                  <Server size={14} style={{ color: "var(--color-accent)", flexShrink: 0 }} />
                  <span style={{ fontSize: 14, fontWeight: 500, color: "var(--color-text-primary)" }}>
                    {svc?.name ?? a.serviceId}
                  </span>
                  {svc && (
                    <span style={{ fontSize: 11, color: "var(--color-text-muted)", background: "var(--color-bg-subtle)", padding: "1px 6px", borderRadius: 3 }}>
                      {svc.type}
                    </span>
                  )}
                  {svc?.status && (
                    <span style={{
                      fontSize: 11,
                      fontWeight: 500,
                      color: svc.status === "online" ? "var(--color-success)" : "var(--color-text-muted)",
                    }}>
                      {svc.status}
                    </span>
                  )}
                  <div style={{ flex: 1 }} />
                  <button
                    onClick={() => unassign.mutate({ indexerId: id!, serviceId: a.serviceId })}
                    style={{ background: "none", border: "none", cursor: "pointer", color: "var(--color-text-muted)", padding: 4 }}
                    title="Remove assignment"
                  >
                    <X size={14} />
                  </button>
                </div>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
}
