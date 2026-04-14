import { useState } from "react";
import { useParams, useNavigate } from "react-router-dom";
import {
  ArrowLeft,
  Trash2,
  Check,
  AlertCircle,
  Loader2,
  Search,
  ExternalLink,
  Heart,
  X,
} from "lucide-react";
import { useConfirm } from "@beacon-shared/ConfirmDialog";
import StatusBadge from "@/components/StatusBadge";
import { card, sectionHeader } from "@/lib/styles";
import { formatDate, timeAgo } from "@/lib/utils";
import { useService, useDeregisterService } from "@/api/services";
import { useIndexersForService, useUnassignIndexer } from "@/api/indexers";
import type { Indexer } from "@/types";

export default function ServiceDetailPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { data: service, isLoading } = useService(id!);
  const { data: indexers } = useIndexersForService(id!);
  const deregister = useDeregisterService();
  const unassign = useUnassignIndexer();
  const confirm = useConfirm();

  const [healthStatus, setHealthStatus] = useState<"idle" | "checking" | "ok" | "fail">("idle");
  const [healthMessage, setHealthMessage] = useState("");

  const handleHealthCheck = async () => {
    if (!service?.health_url) return;
    setHealthStatus("checking");
    setHealthMessage("");
    try {
      const res = await fetch("/api/v1/indexers/test", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ kind: "generic", url: service.health_url, api_key: "" }),
      });
      const data = (await res.json()) as { success: boolean; message: string; duration: string };
      setHealthStatus(data.success ? "ok" : "fail");
      setHealthMessage(data.message + (data.duration ? ` (${data.duration})` : ""));
    } catch {
      setHealthStatus("fail");
      setHealthMessage("Request failed");
    }
  };

  const handleDeregister = async () => {
    if (!service) return;
    if (
      !(await confirm({
        title: "Deregister service",
        message: `Deregister "${service.name}"? It can re-register on next startup.`,
        confirmLabel: "Deregister",
      }))
    )
      return;
    deregister.mutate(service.id, { onSuccess: () => navigate("/services") });
  };

  if (isLoading) {
    return <div style={{ padding: 24, color: "var(--color-text-muted)", fontSize: 13 }}>Loading...</div>;
  }

  if (!service) {
    return (
      <div style={{ padding: 24 }}>
        <div style={{ color: "var(--color-danger)", fontSize: 14 }}>Service not found</div>
        <button onClick={() => navigate("/services")} style={{ marginTop: 12, background: "none", border: "none", color: "var(--color-accent)", cursor: "pointer", fontSize: 13 }}>
          Back to Services
        </button>
      </div>
    );
  }

  // Split capabilities into content vs feature capabilities
  const contentCaps = service.capabilities.filter((c) => c.startsWith("content:"));
  const featureCaps = service.capabilities.filter((c) => !c.startsWith("content:"));

  return (
    <div style={{ padding: 24, maxWidth: 900 }}>
      {/* Back + title */}
      <div style={{ display: "flex", alignItems: "center", gap: 12, marginBottom: 20 }}>
        <button
          onClick={() => navigate("/services")}
          style={{
            display: "flex", alignItems: "center", justifyContent: "center",
            width: 32, height: 32, borderRadius: 6,
            border: "1px solid var(--color-border-default)",
            background: "transparent", cursor: "pointer", color: "var(--color-text-secondary)",
          }}
        >
          <ArrowLeft size={16} />
        </button>
        <div style={{ flex: 1 }}>
          <h1 style={{ margin: 0, fontSize: 20, fontWeight: 600, color: "var(--color-text-primary)" }}>
            {service.name}
          </h1>
          <div style={{ display: "flex", gap: 6, marginTop: 4, alignItems: "center" }}>
            <StatusBadge status={service.status} />
            <span style={{ fontSize: 12, color: "var(--color-text-muted)", background: "var(--color-bg-subtle)", padding: "2px 8px", borderRadius: 4 }}>{service.type}</span>
            {service.version && (
              <span style={{ fontSize: 12, color: "var(--color-text-muted)", fontFamily: "var(--font-family-mono)" }}>{service.version}</span>
            )}
          </div>
        </div>

        <button
          onClick={handleDeregister}
          style={{
            padding: "7px 14px", borderRadius: 6,
            border: "1px solid var(--color-border-default)",
            background: "transparent", color: "var(--color-danger)",
            fontSize: 13, cursor: "pointer",
            display: "flex", alignItems: "center", gap: 4,
          }}
        >
          <Trash2 size={13} /> Deregister
        </button>
      </div>

      {/* Connection info */}
      <div style={{ ...card, marginBottom: 16 }}>
        <h3 style={sectionHeader}>Connection</h3>
        <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: "14px 24px" }}>
          <div>
            <div style={{ fontSize: 11, fontWeight: 600, color: "var(--color-text-muted)", textTransform: "uppercase", letterSpacing: "0.04em", marginBottom: 4 }}>API URL</div>
            <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
              <span style={{ fontSize: 13, color: "var(--color-text-secondary)", fontFamily: "var(--font-family-mono)", wordBreak: "break-all" }}>{service.api_url}</span>
              {service.api_url && (
                <a href={service.api_url} target="_blank" rel="noopener noreferrer" style={{ color: "var(--color-text-muted)", flexShrink: 0 }}>
                  <ExternalLink size={12} />
                </a>
              )}
            </div>
          </div>
          <div>
            <div style={{ fontSize: 11, fontWeight: 600, color: "var(--color-text-muted)", textTransform: "uppercase", letterSpacing: "0.04em", marginBottom: 4 }}>Health URL</div>
            <span style={{ fontSize: 13, color: "var(--color-text-secondary)", fontFamily: "var(--font-family-mono)", wordBreak: "break-all" }}>{service.health_url || "—"}</span>
          </div>
          <div>
            <div style={{ fontSize: 11, fontWeight: 600, color: "var(--color-text-muted)", textTransform: "uppercase", letterSpacing: "0.04em", marginBottom: 4 }}>Registered</div>
            <span style={{ fontSize: 13, color: "var(--color-text-secondary)" }}>{service.registered ? formatDate(service.registered, true) : "—"}</span>
          </div>
          <div>
            <div style={{ fontSize: 11, fontWeight: 600, color: "var(--color-text-muted)", textTransform: "uppercase", letterSpacing: "0.04em", marginBottom: 4 }}>Last Seen</div>
            <span style={{ fontSize: 13, color: "var(--color-text-secondary)" }}>{service.last_seen ? `${timeAgo(service.last_seen)} (${formatDate(service.last_seen)})` : "—"}</span>
          </div>
          <div style={{ gridColumn: "1 / -1" }}>
            <div style={{ fontSize: 11, fontWeight: 600, color: "var(--color-text-muted)", textTransform: "uppercase", letterSpacing: "0.04em", marginBottom: 4 }}>Service ID</div>
            <span style={{ fontSize: 12, color: "var(--color-text-muted)", fontFamily: "var(--font-family-mono)" }}>{service.id}</span>
          </div>
        </div>
      </div>

      {/* Health check */}
      <div style={{ ...card, marginBottom: 16 }}>
        <h3 style={sectionHeader}>Health Check</h3>
        <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
          <button
            onClick={handleHealthCheck}
            disabled={healthStatus === "checking" || !service.health_url}
            style={{
              padding: "7px 16px", borderRadius: 6,
              border: "1px solid var(--color-border-default)",
              background: "var(--color-bg-elevated)",
              color: !service.health_url ? "var(--color-text-muted)" : "var(--color-text-secondary)",
              fontSize: 13, cursor: service.health_url ? "pointer" : "not-allowed",
              display: "flex", alignItems: "center", gap: 6,
              opacity: service.health_url ? 1 : 0.5,
            }}
          >
            {healthStatus === "checking" ? (
              <><Loader2 size={14} style={{ animation: "spin 1s linear infinite" }} /> Checking...</>
            ) : (
              <><Heart size={14} /> Check Health</>
            )}
          </button>
          {!service.health_url && (
            <span style={{ fontSize: 12, color: "var(--color-text-muted)" }}>No health URL configured</span>
          )}
          {healthStatus === "ok" && (
            <span style={{ display: "flex", alignItems: "center", gap: 4, fontSize: 13, color: "var(--color-success)" }}>
              <Check size={14} /> {healthMessage}
            </span>
          )}
          {healthStatus === "fail" && (
            <span style={{ display: "flex", alignItems: "center", gap: 4, fontSize: 13, color: "var(--color-danger)" }}>
              <AlertCircle size={14} /> {healthMessage}
            </span>
          )}
        </div>
      </div>

      {/* Capabilities */}
      <div style={{ ...card, marginBottom: 16 }}>
        <h3 style={sectionHeader}>Capabilities</h3>
        {service.capabilities.length === 0 ? (
          <div style={{ fontSize: 13, color: "var(--color-text-muted)" }}>No capabilities declared.</div>
        ) : (
          <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
            {contentCaps.length > 0 && (
              <div>
                <div style={{ fontSize: 11, color: "var(--color-text-muted)", marginBottom: 6 }}>Content types handled:</div>
                <div style={{ display: "flex", flexWrap: "wrap", gap: 4 }}>
                  {contentCaps.map((cap) => (
                    <span key={cap} style={{ fontSize: 12, padding: "3px 10px", borderRadius: 4, background: "color-mix(in srgb, var(--color-info) 12%, transparent)", color: "var(--color-info)", fontWeight: 500 }}>
                      {cap.replace("content:", "")}
                    </span>
                  ))}
                </div>
              </div>
            )}
            {featureCaps.length > 0 && (
              <div>
                <div style={{ fontSize: 11, color: "var(--color-text-muted)", marginBottom: 6 }}>Features:</div>
                <div style={{ display: "flex", flexWrap: "wrap", gap: 4 }}>
                  {featureCaps.map((cap) => (
                    <span key={cap} style={{ fontSize: 11, padding: "2px 8px", borderRadius: 4, background: "var(--color-accent-muted)", color: "var(--color-accent)", fontFamily: "var(--font-family-mono)" }}>
                      {cap}
                    </span>
                  ))}
                </div>
              </div>
            )}
          </div>
        )}
      </div>

      {/* Assigned indexers */}
      <div style={card}>
        <h3 style={sectionHeader}>Assigned Indexers</h3>
        {!indexers?.length ? (
          <div style={{ fontSize: 13, color: "var(--color-text-muted)" }}>
            No indexers assigned to this service. Indexers are auto-assigned based on content category matching.
          </div>
        ) : (
          <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
            {indexers.map((idx: Indexer) => (
              <div
                key={idx.id}
                style={{
                  display: "flex", alignItems: "center", gap: 10,
                  padding: "8px 12px", borderRadius: 6,
                  border: "1px solid var(--color-border-subtle)",
                  background: "var(--color-bg-elevated)",
                }}
              >
                <Search size={14} style={{ color: "var(--color-accent)", flexShrink: 0 }} />
                <button
                  onClick={() => navigate(`/indexers/${idx.id}`)}
                  style={{ background: "none", border: "none", cursor: "pointer", fontSize: 14, fontWeight: 500, color: "var(--color-text-primary)", padding: 0 }}
                >
                  {idx.name}
                </button>
                <span style={{
                  fontSize: 10, fontWeight: 600, padding: "2px 6px", borderRadius: 3, textTransform: "uppercase",
                  color: idx.kind === "torznab" ? "#3b9eff" : "#f59e0b",
                  background: idx.kind === "torznab" ? "color-mix(in srgb, #3b9eff 12%, transparent)" : "color-mix(in srgb, #f59e0b 12%, transparent)",
                }}>
                  {idx.kind}
                </span>
                <span style={{ fontSize: 12, color: "var(--color-text-muted)", fontFamily: "var(--font-family-mono)", flex: 1, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                  {idx.url}
                </span>
                <button
                  onClick={() => unassign.mutate({ indexerId: idx.id, serviceId: id! })}
                  style={{ background: "none", border: "none", cursor: "pointer", color: "var(--color-text-muted)", padding: 4, flexShrink: 0 }}
                  title="Unassign"
                >
                  <X size={14} />
                </button>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
