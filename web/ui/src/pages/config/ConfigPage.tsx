import { useState } from "react";
import { Plus, Trash2 } from "lucide-react";
import PageHeader from "@/components/PageHeader";
import Modal from "@/components/Modal";
import { useConfigEntries, useSetConfig, useDeleteConfig, useConfigNamespaces } from "@/api/config";
import { card } from "@/lib/styles";
import { formatDate } from "@/lib/utils";

function AddConfigModal({ onClose }: { onClose: () => void }) {
  const setConfig = useSetConfig();
  const [namespace, setNamespace] = useState("");
  const [key, setKey] = useState("");
  const [value, setValue] = useState("");

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
    color: "var(--color-text-secondary)",
    marginBottom: 4,
  };

  return (
    <Modal onClose={onClose} width={440}>
      <div style={{ padding: 20 }}>
        <h2 style={{ margin: "0 0 16px", fontSize: 16, fontWeight: 600, color: "var(--color-text-primary)" }}>
          Add Config Entry
        </h2>
        <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
          <div>
            <label style={labelStyle}>Namespace</label>
            <input style={inputStyle} value={namespace} onChange={(e) => setNamespace(e.target.value)} placeholder="quality" />
          </div>
          <div>
            <label style={labelStyle}>Key</label>
            <input style={inputStyle} value={key} onChange={(e) => setKey(e.target.value)} placeholder="preferred_codec" />
          </div>
          <div>
            <label style={labelStyle}>Value (JSON)</label>
            <textarea
              style={{ ...inputStyle, minHeight: 80, fontFamily: "var(--font-family-mono)", resize: "vertical" }}
              value={value}
              onChange={(e) => setValue(e.target.value)}
              placeholder={'"x265"'}
            />
          </div>
        </div>
        <div style={{ display: "flex", justifyContent: "flex-end", gap: 8, marginTop: 20 }}>
          <button
            onClick={onClose}
            style={{ padding: "7px 16px", borderRadius: 6, border: "1px solid var(--color-border-default)", background: "transparent", color: "var(--color-text-secondary)", fontSize: 13, cursor: "pointer" }}
          >
            Cancel
          </button>
          <button
            onClick={() => setConfig.mutate({ namespace, key, value }, { onSuccess: onClose })}
            disabled={!namespace || !key || !value || setConfig.isPending}
            style={{ padding: "7px 16px", borderRadius: 6, border: "none", background: "var(--color-accent)", color: "var(--color-accent-fg)", fontSize: 13, fontWeight: 500, cursor: "pointer", opacity: !namespace || !key || !value ? 0.5 : 1 }}
          >
            {setConfig.isPending ? "Saving..." : "Save"}
          </button>
        </div>
      </div>
    </Modal>
  );
}

export default function ConfigPage() {
  const { data: entries, isLoading } = useConfigEntries();
  const { data: namespaces } = useConfigNamespaces();
  const deleteConfig = useDeleteConfig();
  const [showAdd, setShowAdd] = useState(false);
  const [filterNs, setFilterNs] = useState<string>("");

  const filtered = filterNs
    ? entries?.filter((e) => e.namespace === filterNs)
    : entries;

  // Group by namespace
  const grouped = new Map<string, typeof entries>();
  for (const entry of filtered ?? []) {
    const list = grouped.get(entry.namespace) ?? [];
    list.push(entry);
    grouped.set(entry.namespace, list);
  }

  return (
    <div style={{ padding: 24, maxWidth: 1200 }}>
      <PageHeader
        title="Shared Config"
        description="Centralized configuration shared across all ecosystem services. Organized by namespace."
        action={
          <button
            onClick={() => setShowAdd(true)}
            style={{ display: "flex", alignItems: "center", gap: 6, padding: "7px 14px", borderRadius: 6, border: "none", background: "var(--color-accent)", color: "var(--color-accent-fg)", fontSize: 13, fontWeight: 500, cursor: "pointer" }}
          >
            <Plus size={15} /> Add Entry
          </button>
        }
      />

      {/* Namespace filter */}
      {namespaces && namespaces.length > 0 && (
        <div style={{ display: "flex", gap: 6, marginBottom: 16, flexWrap: "wrap" }}>
          <button
            onClick={() => setFilterNs("")}
            style={{
              padding: "4px 12px",
              borderRadius: 4,
              border: "1px solid var(--color-border-default)",
              background: !filterNs ? "var(--color-accent-muted)" : "transparent",
              color: !filterNs ? "var(--color-accent)" : "var(--color-text-secondary)",
              fontSize: 12,
              fontWeight: 500,
              cursor: "pointer",
            }}
          >
            All
          </button>
          {namespaces.map((ns) => (
            <button
              key={ns}
              onClick={() => setFilterNs(ns)}
              style={{
                padding: "4px 12px",
                borderRadius: 4,
                border: "1px solid var(--color-border-default)",
                background: filterNs === ns ? "var(--color-accent-muted)" : "transparent",
                color: filterNs === ns ? "var(--color-accent)" : "var(--color-text-secondary)",
                fontSize: 12,
                fontWeight: 500,
                cursor: "pointer",
              }}
            >
              {ns}
            </button>
          ))}
        </div>
      )}

      {isLoading ? (
        <div style={{ color: "var(--color-text-muted)", fontSize: 13 }}>Loading...</div>
      ) : !filtered?.length ? (
        <div style={{ ...card, color: "var(--color-text-muted)", fontSize: 13, textAlign: "center", padding: 40 }}>
          No config entries. Add shared configuration like quality profiles, naming conventions, and categories.
        </div>
      ) : (
        Array.from(grouped.entries()).map(([ns, nsEntries]) => (
          <div key={ns} style={{ marginBottom: 24 }}>
            <h3 style={{ margin: "0 0 8px", fontSize: 12, fontWeight: 600, color: "var(--color-text-muted)", textTransform: "uppercase", letterSpacing: "0.06em" }}>
              {ns}
            </h3>
            <div style={{ ...card, padding: 0, overflow: "hidden" }}>
              <table style={{ width: "100%", borderCollapse: "collapse" }}>
                <thead>
                  <tr style={{ borderBottom: "1px solid var(--color-border-subtle)" }}>
                    {["Key", "Value", "Updated", ""].map((h) => (
                      <th key={h} style={{ textAlign: "left", padding: "8px 16px", fontSize: 11, fontWeight: 600, color: "var(--color-text-muted)", textTransform: "uppercase", letterSpacing: "0.06em" }}>
                        {h}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {nsEntries!.map((entry) => (
                    <tr key={entry.key} style={{ borderBottom: "1px solid var(--color-border-subtle)" }}>
                      <td style={{ padding: "8px 16px", fontSize: 13, fontWeight: 500, color: "var(--color-text-primary)", fontFamily: "var(--font-family-mono)" }}>
                        {entry.key}
                      </td>
                      <td style={{ padding: "8px 16px", fontSize: 13, color: "var(--color-text-secondary)", fontFamily: "var(--font-family-mono)", maxWidth: 300, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                        {entry.value}
                      </td>
                      <td style={{ padding: "8px 16px", fontSize: 12, color: "var(--color-text-muted)" }}>
                        {formatDate(entry.updated_at)}
                      </td>
                      <td style={{ padding: "8px 16px", textAlign: "right" }}>
                        <button
                          onClick={() => deleteConfig.mutate({ namespace: entry.namespace, key: entry.key })}
                          style={{ background: "none", border: "none", cursor: "pointer", color: "var(--color-text-muted)", padding: 4 }}
                          title="Delete"
                        >
                          <Trash2 size={14} />
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        ))
      )}

      {showAdd && <AddConfigModal onClose={() => setShowAdd(false)} />}
    </div>
  );
}
