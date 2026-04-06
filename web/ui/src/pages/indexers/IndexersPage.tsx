import { useState, useCallback } from "react";
import { useNavigate } from "react-router-dom";
import { Plus, ChevronRight, FlaskConical, Check, AlertCircle, Loader2, Trash2, CheckSquare } from "lucide-react";
import PageHeader from "@/components/PageHeader";
import Pill from "@/components/Pill";
import { useIndexers, useDeleteIndexer } from "@/api/indexers";
import { useCatalog } from "@/api/catalog";
import { card } from "@/lib/styles";
import type { Indexer } from "@/types";

const categoryColors: Record<string, string> = {
  Movies: "#3b9eff",
  TV: "#34d399",
  Audio: "#f59e0b",
  Books: "#a78bfa",
  XXX: "#f87171",
  Other: "#6b7280",
};

type TestStatus = "idle" | "testing" | "pass" | "fail" | "cloudflare";

interface TestState {
  status: TestStatus;
  message: string;
}

export default function IndexersPage() {
  const navigate = useNavigate();
  const { data: indexers, isLoading } = useIndexers();
  const deleteIndexer = useDeleteIndexer();
  const { data: catalogData } = useCatalog();

  // Build a name→categories lookup from the catalog
  const catsByName = new Map<string, string[]>();
  if (catalogData?.entries) {
    for (const e of catalogData.entries) {
      catsByName.set(e.name.toLowerCase(), e.categories);
    }
  }
  const [testStates, setTestStates] = useState<Record<string, TestState>>({});
  const [testingAll, setTestingAll] = useState(false);
  const [selectMode, setSelectMode] = useState(false);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [deleting, setDeleting] = useState(false);

  const enterSelectMode = () => {
    setSelectMode(true);
    setSelected(new Set());
  };

  const exitSelectMode = () => {
    setSelectMode(false);
    setSelected(new Set());
  };

  const toggleSelect = (id: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id); else next.add(id);
      return next;
    });
  };

  const toggleAll = () => {
    if (!indexers) return;
    if (selected.size === indexers.length) {
      setSelected(new Set());
    } else {
      setSelected(new Set(indexers.map((i) => i.id)));
    }
  };

  const deleteSelected = async () => {
    if (selected.size === 0) return;
    if (!confirm(`Delete ${selected.size} indexer${selected.size > 1 ? "s" : ""}? This cannot be undone.`)) return;
    setDeleting(true);
    for (const id of selected) {
      await deleteIndexer.mutateAsync(id).catch(() => {});
    }
    setSelected(new Set());
    setDeleting(false);
    setSelectMode(false);
  };

  const testOne = useCallback(async (idx: Indexer) => {
    setTestStates((prev) => ({ ...prev, [idx.id]: { status: "testing", message: "" } }));
    try {
      const res = await fetch(`/api/v1/indexers/${idx.id}/test-search`, { method: "POST" });
      const data = (await res.json()) as { success: boolean; message: string; results: number; cloudflare?: boolean };
      setTestStates((prev) => ({
        ...prev,
        [idx.id]: {
          status: data.cloudflare ? "cloudflare" : data.success ? "pass" : "fail",
          message: data.message,
        },
      }));
    } catch {
      setTestStates((prev) => ({
        ...prev,
        [idx.id]: { status: "fail", message: "Request failed" },
      }));
    }
  }, []);

  const testAll = useCallback(async () => {
    if (!indexers?.length || testingAll) return;
    setTestingAll(true);
    const initial: Record<string, TestState> = {};
    for (const idx of indexers) {
      initial[idx.id] = { status: "testing", message: "" };
    }
    setTestStates(initial);
    await Promise.all(indexers.map((idx) => testOne(idx)));
    setTestingAll(false);
  }, [indexers, testingAll, testOne]);

  const passCount = Object.values(testStates).filter((t) => t.status === "pass").length;
  const failCount = Object.values(testStates).filter((t) => t.status === "fail").length;
  const cfCount = Object.values(testStates).filter((t) => t.status === "cloudflare").length;
  const testedCount = passCount + failCount + cfCount;
  const hasIndexers = indexers && indexers.length > 0;

  return (
    <div style={{ padding: 24, maxWidth: 1200 }}>
      <PageHeader
        title="Indexers"
        description="Centrally managed indexers. Auto-assigned to services based on content categories."
        action={
          <div style={{ display: "flex", gap: 8 }}>
            {hasIndexers && !selectMode && (
              <>
                <button
                  onClick={enterSelectMode}
                  style={{
                    display: "flex", alignItems: "center", gap: 6,
                    padding: "7px 14px", borderRadius: 6,
                    border: "1px solid var(--color-border-default)",
                    background: "transparent", color: "var(--color-text-secondary)",
                    fontSize: 13, fontWeight: 500, cursor: "pointer",
                  }}
                >
                  <CheckSquare size={14} /> Select
                </button>
                <button
                  onClick={testAll}
                  disabled={testingAll}
                  style={{
                    display: "flex", alignItems: "center", gap: 6,
                    padding: "7px 14px", borderRadius: 6,
                    border: "1px solid var(--color-border-default)",
                    background: "transparent", color: "var(--color-text-secondary)",
                    fontSize: 13, fontWeight: 500,
                    cursor: testingAll ? "wait" : "pointer",
                  }}
                >
                  {testingAll ? (
                    <><Loader2 size={14} style={{ animation: "spin 1s linear infinite" }} /> Testing...</>
                  ) : (
                    <><FlaskConical size={14} /> Test All</>
                  )}
                </button>
              </>
            )}
            {selectMode && (
              <button
                onClick={exitSelectMode}
                style={{
                  display: "flex", alignItems: "center", gap: 6,
                  padding: "7px 14px", borderRadius: 6,
                  border: "1px solid var(--color-border-default)",
                  background: "transparent", color: "var(--color-text-secondary)",
                  fontSize: 13, fontWeight: 500, cursor: "pointer",
                }}
              >
                Done
              </button>
            )}
            <button
              onClick={() => navigate("/indexers/add")}
              style={{ display: "flex", alignItems: "center", gap: 6, padding: "7px 14px", borderRadius: 6, border: "none", background: "var(--color-accent)", color: "var(--color-accent-fg)", fontSize: 13, fontWeight: 500, cursor: "pointer" }}
            >
              <Plus size={15} /> Add Indexer
            </button>
          </div>
        }
      />

      {/* Selection action bar */}
      {selectMode && (
        <div style={{
          display: "flex", alignItems: "center", gap: 12,
          marginBottom: 12, padding: "8px 14px", borderRadius: 6,
          background: "var(--color-bg-elevated)",
          border: "1px solid var(--color-border-default)",
          fontSize: 13,
        }}>
          <input
            type="checkbox"
            checked={hasIndexers && selected.size === indexers!.length}
            ref={(el) => { if (el) el.indeterminate = selected.size > 0 && selected.size < (indexers?.length ?? 0); }}
            onChange={toggleAll}
            style={{ cursor: "pointer", width: 15, height: 15, accentColor: "var(--color-accent)" }}
          />
          <span style={{ color: "var(--color-text-secondary)", fontWeight: 500 }}>
            {selected.size > 0 ? `${selected.size} selected` : "Select all"}
          </span>
          {selected.size > 0 && (
            <button
              onClick={deleteSelected}
              disabled={deleting}
              style={{
                display: "flex", alignItems: "center", gap: 4,
                padding: "4px 12px", borderRadius: 5,
                border: "1px solid color-mix(in srgb, var(--color-danger) 40%, transparent)",
                background: "color-mix(in srgb, var(--color-danger) 8%, transparent)",
                color: "var(--color-danger)",
                fontSize: 12, fontWeight: 500, cursor: "pointer",
              }}
            >
              <Trash2 size={12} /> {deleting ? "Deleting..." : `Delete ${selected.size}`}
            </button>
          )}
        </div>
      )}

      {/* Test summary bar */}
      {testedCount > 0 && !selectMode && (
        <div style={{
          display: "flex", gap: 16, marginBottom: 16,
          padding: "8px 14px", borderRadius: 6,
          background: "var(--color-bg-surface)",
          border: "1px solid var(--color-border-subtle)",
          fontSize: 13,
        }}>
          <span style={{ color: "var(--color-text-secondary)" }}>Test results:</span>
          <span style={{ color: "var(--color-success)", display: "flex", alignItems: "center", gap: 4 }}>
            <Check size={13} /> {passCount} passed
          </span>
          {failCount > 0 && (
            <span style={{ color: "var(--color-danger)", display: "flex", alignItems: "center", gap: 4 }}>
              <AlertCircle size={13} /> {failCount} failed
            </span>
          )}
          {cfCount > 0 && (
            <span style={{ color: "var(--color-warning)", display: "flex", alignItems: "center", gap: 4 }}>
              ☁ {cfCount} Cloudflare blocked
            </span>
          )}
          {testingAll && (
            <span style={{ color: "var(--color-text-muted)" }}>
              {testedCount} of {indexers?.length ?? 0} tested...
            </span>
          )}
        </div>
      )}

      {isLoading ? (
        <div style={{ color: "var(--color-text-muted)", fontSize: 13 }}>Loading...</div>
      ) : !hasIndexers ? (
        <div style={{ ...card, color: "var(--color-text-muted)", fontSize: 13, textAlign: "center", padding: 40 }}>
          No indexers configured. Add indexers to manage them centrally across all services.
        </div>
      ) : (
        <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
          {indexers.map((idx) => {
            const test = testStates[idx.id];
            const isSelected = selected.has(idx.id);
            return (
              <div
                key={idx.id}
                onClick={() => {
                  if (selectMode) toggleSelect(idx.id);
                  else navigate(`/indexers/${idx.id}`);
                }}
                style={{
                  display: "flex", alignItems: "center", gap: 12,
                  padding: "12px 16px",
                  borderRadius: 8,
                  border: test?.status === "fail"
                    ? "1px solid color-mix(in srgb, var(--color-danger) 30%, transparent)"
                    : test?.status === "cloudflare"
                    ? "1px solid color-mix(in srgb, var(--color-warning) 30%, transparent)"
                    : test?.status === "pass"
                    ? "1px solid color-mix(in srgb, var(--color-success) 30%, transparent)"
                    : isSelected
                    ? "1px solid var(--color-accent)"
                    : "1px solid var(--color-border-subtle)",
                  background: isSelected ? "var(--color-accent-muted)" : "var(--color-bg-surface)",
                  cursor: "pointer",
                  transition: "border-color 120ms ease, background 120ms ease",
                }}
              >
                {/* Checkbox — only in select mode */}
                {selectMode && (
                  <input
                    type="checkbox"
                    checked={isSelected}
                    readOnly
                    style={{ cursor: "pointer", width: 15, height: 15, accentColor: "var(--color-accent)", flexShrink: 0 }}
                  />
                )}

                <TestIndicator status={test?.status ?? "idle"} />

                <div style={{ flex: 1, minWidth: 0 }}>
                  <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                    <span style={{ fontSize: 15, fontWeight: 600, color: "var(--color-text-primary)" }}>{idx.name}</span>
                    <span style={{
                      fontSize: 10, fontWeight: 600, padding: "2px 6px", borderRadius: 3,
                      textTransform: "uppercase", letterSpacing: "0.04em",
                      color: idx.kind === "torznab" ? "#3b9eff" : "#f59e0b",
                      background: idx.kind === "torznab"
                        ? "color-mix(in srgb, #3b9eff 12%, transparent)"
                        : "color-mix(in srgb, #f59e0b 12%, transparent)",
                    }}>
                      {idx.kind}
                    </span>
                    <Pill ok={idx.enabled} labelTrue="Enabled" labelFalse="Disabled" />
                    {(catsByName.get(idx.name.toLowerCase()) ?? []).map((cat) => (
                      <span key={cat} style={{
                        fontSize: 10, fontWeight: 500, padding: "1px 6px", borderRadius: 3,
                        color: categoryColors[cat] ?? "#6b7280",
                        background: `color-mix(in srgb, ${categoryColors[cat] ?? "#6b7280"} 10%, transparent)`,
                      }}>
                        {cat}
                      </span>
                    ))}
                  </div>
                  <div style={{ display: "flex", alignItems: "center", gap: 8, marginTop: 4 }}>
                    <span style={{ fontSize: 12, color: "var(--color-text-muted)", fontFamily: "var(--font-family-mono)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                      {idx.url}
                    </span>
                    {test?.message && (
                      <span style={{
                        fontSize: 11,
                        color: test.status === "pass" ? "var(--color-success)"
                          : test.status === "cloudflare" ? "var(--color-warning)"
                          : test.status === "fail" ? "var(--color-danger)"
                          : "var(--color-text-muted)",
                        whiteSpace: "nowrap",
                      }}>
                        {test.message}
                      </span>
                    )}
                  </div>
                </div>

                <span style={{ fontSize: 12, color: "var(--color-text-muted)", fontFamily: "var(--font-family-mono)", flexShrink: 0 }}>
                  P{idx.priority}
                </span>
                {!selectMode && <ChevronRight size={16} style={{ color: "var(--color-text-muted)", flexShrink: 0 }} />}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

function TestIndicator({ status }: { status: TestStatus }) {
  switch (status) {
    case "testing":
      return <Loader2 size={16} style={{ color: "var(--color-accent)", animation: "spin 1s linear infinite", flexShrink: 0 }} />;
    case "pass":
      return (
        <div style={{ width: 20, height: 20, borderRadius: "50%", flexShrink: 0, background: "color-mix(in srgb, var(--color-success) 15%, transparent)", display: "flex", alignItems: "center", justifyContent: "center" }}>
          <Check size={12} style={{ color: "var(--color-success)" }} />
        </div>
      );
    case "cloudflare":
      return (
        <div style={{ width: 20, height: 20, borderRadius: "50%", flexShrink: 0, background: "color-mix(in srgb, var(--color-warning) 15%, transparent)", display: "flex", alignItems: "center", justifyContent: "center", fontSize: 11 }}>
          ☁
        </div>
      );
    case "fail":
      return (
        <div style={{ width: 20, height: 20, borderRadius: "50%", flexShrink: 0, background: "color-mix(in srgb, var(--color-danger) 15%, transparent)", display: "flex", alignItems: "center", justifyContent: "center" }}>
          <AlertCircle size={12} style={{ color: "var(--color-danger)" }} />
        </div>
      );
    default:
      return (
        <div style={{ width: 20, height: 20, borderRadius: "50%", flexShrink: 0, background: "var(--color-bg-subtle)", display: "flex", alignItems: "center", justifyContent: "center" }}>
          <div style={{ width: 6, height: 6, borderRadius: "50%", background: "var(--color-text-muted)" }} />
        </div>
      );
  }
}
