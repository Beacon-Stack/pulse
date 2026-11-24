import { useState } from "react";
import { Link } from "react-router-dom";
import { X, Check, AlertCircle, Loader2, FlaskConical } from "lucide-react";
import { useTestCatalogIndexer } from "@/api/catalog";
import { VerdictBadge, HealthLine } from "@/components/CatalogHealthBadge";
import { timeAgo, timeUntil, verdictInfo } from "@/lib/catalogHealth";
import { useCreateIndexer } from "@/api/indexers";
import { toast } from "sonner";
import type { CatalogEntry, CatalogField, CatalogSiteHealth, CatalogHealthResponse } from "@/types";
import { ProtocolBadge, PrivacyBadge, categoryColors } from "./IndexerCatalogCards";

export default function ConfigDrawer({
  entry,
  isAdded,
  addedIndexerId,
  health,
  onClose,
}: {
  entry: CatalogEntry;
  isAdded: boolean;
  addedIndexerId?: string;
  health?: CatalogSiteHealth;
  onClose: () => void;
}) {
  const createIndexer = useCreateIndexer();
  const [fieldValues, setFieldValues] = useState<Record<string, string>>(() => {
    const initial: Record<string, string> = {};
    for (const f of entry.settings) {
      initial[f.name] = f.default ?? "";
    }
    return initial;
  });
  const [testStatus, setTestStatus] = useState<"idle" | "testing" | "success" | "error">("idle");

  const setField = (name: string, value: string) => {
    setFieldValues((prev) => ({ ...prev, [name]: value }));
  };

  const [testMessage, setTestMessage] = useState("");

  const handleTest = async () => {
    setTestStatus("testing");
    setTestMessage("");
    try {
      // If the user provided a URL field (generic torznab/newznab), use it as
      // an API endpoint and test with a caps query. Otherwise the URL is just
      // the tracker's homepage — do a simple connectivity check.
      const userProvidedUrl = fieldValues["url"];
      const url = userProvidedUrl || entry.urls[0] || "";
      const apiKey = fieldValues["api_key"] || fieldValues["passkey"] || "";
      const kind = userProvidedUrl
        ? (entry.protocol === "usenet" ? "newznab" : "torznab")
        : "generic";
      const res = await fetch("/api/v1/indexers/test", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ kind, url, api_key: apiKey }),
      });
      const data = await res.json() as { success: boolean; message: string; duration: string };
      setTestStatus(data.success ? "success" : "error");
      setTestMessage(data.message + (data.duration ? ` (${data.duration})` : ""));
    } catch {
      setTestStatus("error");
      setTestMessage("Request failed — is Pulse running?");
    }
  };

  const handleSave = () => {
    const url = fieldValues["url"] || entry.urls[0] || "";
    const apiKey = fieldValues["api_key"] || fieldValues["passkey"] || "";

    // Build settings JSON from non-standard fields
    const settingsObj: Record<string, string> = {};
    for (const f of entry.settings) {
      if (f.name !== "url" && f.name !== "api_key" && fieldValues[f.name]) {
        settingsObj[f.name] = fieldValues[f.name];
      }
    }

    createIndexer.mutate(
      {
        name: entry.name,
        kind: entry.protocol === "usenet" ? "newznab" : "torznab",
        url,
        api_key: apiKey,
        settings: JSON.stringify(settingsObj),
        enabled: true,
        priority: 25,
      },
      {
        onSuccess: () => {
          toast.success(`Added ${entry.name}`);
          onClose();
        },
      }
    );
  };

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

  return (
    <>
      {/* Backdrop */}
      <div
        onClick={onClose}
        style={{ position: "fixed", inset: 0, background: "rgba(0,0,0,0.4)", zIndex: 100 }}
      />

      {/* Drawer */}
      <div
        style={{
          position: "fixed",
          top: 0,
          right: 0,
          width: 440,
          maxWidth: "100vw",
          height: "100vh",
          background: "var(--color-bg-surface)",
          borderLeft: "1px solid var(--color-border-subtle)",
          zIndex: 101,
          display: "flex",
          flexDirection: "column",
          boxShadow: "-8px 0 32px rgba(0,0,0,0.3)",
        }}
      >
        {/* Header */}
        <div style={{ padding: "16px 20px", borderBottom: "1px solid var(--color-border-subtle)", display: "flex", alignItems: "center", gap: 12 }}>
          <div style={{ flex: 1 }}>
            <h2 style={{ margin: 0, fontSize: 16, fontWeight: 600, color: "var(--color-text-primary)" }}>{entry.name}</h2>
            <div style={{ display: "flex", gap: 4, marginTop: 4 }}>
              <ProtocolBadge protocol={entry.protocol} />
              <PrivacyBadge privacy={entry.privacy} />
            </div>
          </div>
          <button onClick={onClose} style={{ background: "none", border: "none", cursor: "pointer", color: "var(--color-text-muted)", padding: 4 }}>
            <X size={18} />
          </button>
        </div>

        {/* Body */}
        <div style={{ flex: 1, overflowY: "auto", padding: 20 }}>
          <p style={{ margin: "0 0 16px", fontSize: 13, color: "var(--color-text-secondary)", lineHeight: 1.5 }}>
            {entry.description}
          </p>

          {/* Categories */}
          <div style={{ display: "flex", flexWrap: "wrap", gap: 4, marginBottom: 20 }}>
            {entry.categories.map((cat) => (
              <span key={cat} style={{ fontSize: 11, fontWeight: 500, padding: "2px 8px", borderRadius: 4, color: categoryColors[cat] ?? "#6b7280", background: `color-mix(in srgb, ${categoryColors[cat] ?? "#6b7280"} 12%, transparent)` }}>
                {cat}
              </span>
            ))}
          </div>

          {!isAdded && entry.privacy === "public" && <TestResults entryId={entry.id} health={health} />}

          {/* URLs */}
          {entry.urls.length > 0 && (
            <div style={{ marginBottom: 20 }}>
              <div style={{ fontSize: 11, fontWeight: 600, color: "var(--color-text-muted)", textTransform: "uppercase", letterSpacing: "0.06em", marginBottom: 6 }}>Site URL</div>
              {entry.urls.map((url) => (
                <div key={url} style={{ fontSize: 12, color: "var(--color-text-secondary)", fontFamily: "var(--font-family-mono)", marginBottom: 2 }}>{url}</div>
              ))}
            </div>
          )}

          {/* Config fields */}
          {entry.settings.length > 0 && (
            <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
              <div style={{ fontSize: 11, fontWeight: 600, color: "var(--color-text-muted)", textTransform: "uppercase", letterSpacing: "0.06em" }}>Configuration</div>

              {entry.settings.filter((f) => f.type !== "info").map((field) => (
                <FieldInput key={field.name} field={field} value={fieldValues[field.name] ?? ""} onChange={(v) => setField(field.name, v)} inputStyle={inputStyle} />
              ))}
            </div>
          )}

          {/* Already added notice */}
          {isAdded && (
            <div style={{ marginTop: 20, padding: 12, borderRadius: 6, background: "color-mix(in srgb, var(--color-success) 8%, transparent)", border: "1px solid color-mix(in srgb, var(--color-success) 20%, transparent)" }}>
              <div style={{ display: "flex", alignItems: "center", gap: 6, fontSize: 13, fontWeight: 500, color: "var(--color-success)" }}>
                <Check size={14} /> This indexer has already been added
              </div>
              <p style={{ margin: "4px 0 0", fontSize: 12, color: "var(--color-text-secondary)" }}>
                {addedIndexerId ? (
                  <>
                    <Link to={`/indexers/${addedIndexerId}`} style={{ color: "var(--color-accent)" }}>
                      Open its settings and stats
                    </Link>
                    , or add another instance with a different configuration below.
                  </>
                ) : (
                  "You can add another instance with a different configuration if needed."
                )}
              </p>
            </div>
          )}
        </div>

        {/* Footer */}
        <div style={{ padding: "12px 20px", borderTop: "1px solid var(--color-border-subtle)", display: "flex", alignItems: "center", gap: 8 }}>
          {/* Test status */}
          <div style={{ flex: 1, fontSize: 12 }}>
            {testStatus === "success" && (
              <span style={{ display: "flex", alignItems: "center", gap: 4, color: "var(--color-success)" }}>
                <Check size={14} /> {testMessage || "Connection successful"}
              </span>
            )}
            {testStatus === "error" && (
              <span style={{ display: "flex", alignItems: "center", gap: 4, color: "var(--color-danger)" }}>
                <AlertCircle size={14} /> {testMessage || "Test failed"}
              </span>
            )}
          </div>

          <button
            onClick={handleTest}
            disabled={testStatus === "testing"}
            style={{
              padding: "7px 14px",
              borderRadius: 6,
              border: "1px solid var(--color-border-default)",
              background: "transparent",
              color: "var(--color-text-secondary)",
              fontSize: 13,
              cursor: "pointer",
              display: "flex",
              alignItems: "center",
              gap: 5,
            }}
          >
            {testStatus === "testing" ? (
              <><Loader2 size={13} style={{ animation: "spin 1s linear infinite" }} /> Testing...</>
            ) : (
              "Test"
            )}
          </button>
          <button
            onClick={handleSave}
            disabled={createIndexer.isPending}
            style={{
              padding: "7px 14px",
              borderRadius: 6,
              border: "none",
              background: "var(--color-accent)",
              color: "var(--color-accent-fg)",
              fontSize: 13,
              fontWeight: 500,
              cursor: "pointer",
            }}
          >
            {createIndexer.isPending ? "Saving..." : "Save"}
          </button>
        </div>
      </div>
    </>
  );
}

// Field Input

function FieldInput({
  field,
  value,
  onChange,
  inputStyle,
}: {
  field: CatalogField;
  value: string;
  onChange: (v: string) => void;
  inputStyle: React.CSSProperties;
}) {
  return (
    <div>
      <label style={{ display: "block", fontSize: 12, fontWeight: 500, color: "var(--color-text-secondary)", marginBottom: 4 }}>
        {field.label}
        {field.required && <span style={{ color: "var(--color-danger)", marginLeft: 2 }}>*</span>}
      </label>

      {field.type === "select" ? (
        <select style={inputStyle} value={value} onChange={(e) => onChange(e.target.value)}>
          {field.options?.map((o) => (
            <option key={o.value} value={o.value}>{o.name}</option>
          ))}
        </select>
      ) : field.type === "checkbox" ? (
        <label style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 13, color: "var(--color-text-secondary)", cursor: "pointer" }}>
          <input type="checkbox" checked={value === "true"} onChange={(e) => onChange(e.target.checked ? "true" : "false")} />
          {field.help_text || field.label}
        </label>
      ) : (
        <input
          type={field.type === "password" ? "password" : "text"}
          style={inputStyle}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder={field.placeholder || ""}
        />
      )}

      {field.help_text && field.type !== "checkbox" && (
        <div style={{ fontSize: 11, color: "var(--color-text-muted)", marginTop: 3 }}>{field.help_text}</div>
      )}
    </div>
  );
}

// Background tester

export function TesterStatusLine({ data }: { data: CatalogHealthResponse }) {
  const st = data.status;
  const wanted = st.wanted_categories ?? [];
  const scope = wanted.length > 0 ? `Public ${wanted.join(" and ")} indexers` : "Public indexers in your categories";
  let progress: string;
  if (st.running) {
    progress = `Testing now: ${st.done} of ${st.total} checked${st.query ? `, searching for “${st.query}”` : ""}.`;
  } else if (!st.last_cycle_ended_at) {
    progress = st.next_cycle_at ? `First tests start ${timeUntil(st.next_cycle_at)}.` : "";
  } else {
    progress = `Last round ${timeAgo(st.last_cycle_ended_at)}; next ${timeUntil(st.next_cycle_at)}.`;
  }
  const sites = data.sites ?? [];
  const working = sites.filter((s) => s.verdict === "works").length;
  const eligibleCount = (data.eligible ?? []).length;
  const tested = sites.filter((s) => (data.eligible ?? []).includes(s.catalog_id)).length;
  return (
    <p style={{ margin: "6px 0 0", fontSize: 12, color: "var(--color-text-muted)", display: "flex", alignItems: "center", gap: 6, flexWrap: "wrap" }}>
      <FlaskConical size={13} />
      <span>
        {scope} are test-searched every {st.interval_hours} hours.{" "}
        {sites.length > 0 &&
          (tested < eligibleCount
            ? `${tested} of ${eligibleCount} tested so far, ${working} work. `
            : `${working} of ${eligibleCount} work. `)}
        {progress}
      </span>
    </p>
  );
}

function TestResults({ entryId, health }: { entryId: string; health?: CatalogSiteHealth }) {
  const test = useTestCatalogIndexer();
  const sectionTitle: React.CSSProperties = { fontSize: 11, fontWeight: 600, color: "var(--color-text-muted)", textTransform: "uppercase", letterSpacing: "0.06em", marginBottom: 6 };
  return (
    <div style={{ marginBottom: 20, padding: 12, borderRadius: 8, border: "1px solid var(--color-border-subtle)", background: "var(--color-bg-base)" }}>
      <div style={{ display: "flex", alignItems: "center", gap: 8, ...sectionTitle }}>
        <span style={{ flex: 1 }}>Test results</span>
        <button
          onClick={() => test.mutate(entryId)}
          disabled={test.isPending}
          title="Run one test search now. Sites behind Cloudflare can take a minute or two."
          style={{ display: "inline-flex", alignItems: "center", gap: 4, padding: "4px 10px", borderRadius: 6, border: "1px solid var(--color-border-default)", background: "var(--color-bg-elevated)", color: "var(--color-text-secondary)", fontSize: 12, fontWeight: 500, textTransform: "none", letterSpacing: 0, cursor: test.isPending ? "wait" : "pointer" }}
        >
          {test.isPending ? <Loader2 size={12} style={{ animation: "spin 1s linear infinite" }} /> : <FlaskConical size={12} />}
          {test.isPending ? "Testing..." : "Test now"}
        </button>
      </div>
      {health ? (
        <div style={{ display: "flex", flexDirection: "column", gap: 6, fontSize: 12, color: "var(--color-text-secondary)" }}>
          <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
            <VerdictBadge health={health} />
            <span>{verdictInfo[health.verdict].help}</span>
          </div>
          <HealthLine health={health} />
          <div style={{ color: "var(--color-text-muted)" }}>
            {health.tests} test{health.tests === 1 ? "" : "s"} in the last 30 days, {health.failures} failed. Last test {timeAgo(health.last_test_at)}.
          </div>
          {health.last_error && health.consecutive_failures > 0 && (
            <div style={{ color: "var(--color-danger)", fontFamily: "var(--font-family-mono)", fontSize: 11, wordBreak: "break-word" }}>{health.last_error}</div>
          )}
        </div>
      ) : (
        <div style={{ fontSize: 12, color: "var(--color-text-muted)" }}>
          Not tested yet. Test it now, or wait for the next background round.
        </div>
      )}
    </div>
  );
}
