import { useState, useRef, useEffect } from "react";
import { Bookmark, X, Plus, ChevronDown } from "lucide-react";
import { usePresets, useSavePreset, useDeletePreset } from "@/api/presets";

export interface FilterPreset {
  protocols: string[];
  privacies: string[];
  categories: string[];
  languages: string[];
  search: string;
}

interface Props {
  currentFilters: () => FilterPreset;
  onApply: (preset: FilterPreset, name: string) => void;
  activePresetName: string | null;
}

export default function PresetDropdown({ currentFilters, onApply, activePresetName }: Props) {
  const [open, setOpen] = useState(false);
  const [saving, setSaving] = useState(false);
  const [newName, setNewName] = useState("");
  const ref = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  const { data: presetRows } = usePresets();
  const savePreset = useSavePreset();
  const deletePreset = useDeletePreset();

  // Close on click outside
  useEffect(() => {
    if (!open) return;
    function handleClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false);
        setSaving(false);
      }
    }
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, [open]);

  // Focus input when save mode opens
  useEffect(() => {
    if (saving && inputRef.current) inputRef.current.focus();
  }, [saving]);

  const presets = (presetRows ?? []).map((p) => ({
    id: p.id,
    name: p.name,
    filters: JSON.parse(p.filters) as FilterPreset,
  }));

  const handleSave = () => {
    const name = newName.trim();
    if (!name) return;
    const filters = currentFilters();
    savePreset.mutate(
      { name, filters: JSON.stringify(filters) },
      {
        onSuccess: () => {
          setNewName("");
          setSaving(false);
        },
      }
    );
  };

  const handleDelete = (id: string, e: React.MouseEvent) => {
    e.stopPropagation();
    deletePreset.mutate(id);
  };

  const handleApply = (preset: { name: string; filters: FilterPreset }) => {
    onApply(preset.filters, preset.name);
    setOpen(false);
  };

  const label = activePresetName ?? "Presets";

  return (
    <div ref={ref} style={{ position: "relative" }}>
      {/* Trigger button */}
      <button
        onClick={() => setOpen(!open)}
        style={{
          display: "inline-flex",
          alignItems: "center",
          gap: 5,
          padding: "5px 12px",
          borderRadius: 6,
          border: activePresetName
            ? "1px solid var(--color-accent)"
            : "1px solid var(--color-border-default)",
          background: activePresetName ? "var(--color-accent-muted)" : "transparent",
          color: activePresetName ? "var(--color-accent)" : "var(--color-text-secondary)",
          fontSize: 12,
          fontWeight: 500,
          cursor: "pointer",
          whiteSpace: "nowrap",
          transition: "all 120ms ease",
        }}
      >
        <Bookmark size={13} />
        {label}
        <ChevronDown size={11} style={{ opacity: 0.6 }} />
      </button>

      {/* Dropdown */}
      {open && (
        <div
          style={{
            position: "absolute",
            top: "calc(100% + 4px)",
            left: 0,
            zIndex: 100,
            minWidth: 240,
            maxWidth: 320,
            background: "var(--color-bg-surface)",
            border: "1px solid var(--color-border-default)",
            borderRadius: 8,
            boxShadow: "var(--shadow-modal)",
            overflow: "hidden",
          }}
        >
          {/* Preset list */}
          {presets.length === 0 && !saving && (
            <div style={{ padding: "12px 14px", fontSize: 12, color: "var(--color-text-muted)" }}>
              No saved presets
            </div>
          )}

          {presets.map((p) => (
            <div
              key={p.name}
              onClick={() => handleApply(p)}
              style={{
                display: "flex",
                alignItems: "center",
                gap: 8,
                padding: "8px 14px",
                cursor: "pointer",
                fontSize: 13,
                color: activePresetName === p.name ? "var(--color-accent)" : "var(--color-text-primary)",
                background: activePresetName === p.name ? "var(--color-accent-muted)" : "transparent",
                transition: "background 100ms ease",
              }}
              onMouseEnter={(e) => {
                if (activePresetName !== p.name) e.currentTarget.style.background = "var(--color-bg-elevated)";
              }}
              onMouseLeave={(e) => {
                if (activePresetName !== p.name) e.currentTarget.style.background = "transparent";
              }}
            >
              <Bookmark size={13} style={{ flexShrink: 0, opacity: 0.5 }} />
              <span style={{ flex: 1, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                {p.name}
              </span>
              <FilterSummary filters={p.filters} />
              <button
                onClick={(e) => handleDelete(p.id, e)}
                style={{
                  background: "none",
                  border: "none",
                  cursor: "pointer",
                  color: "var(--color-text-muted)",
                  padding: 2,
                  flexShrink: 0,
                  display: "flex",
                }}
                title="Delete preset"
              >
                <X size={12} />
              </button>
            </div>
          ))}

          {/* Divider */}
          <div style={{ height: 1, background: "var(--color-border-subtle)" }} />

          {/* Save section */}
          {saving ? (
            <div style={{ padding: "8px 14px", display: "flex", gap: 6 }}>
              <input
                ref={inputRef}
                type="text"
                value={newName}
                onChange={(e) => setNewName(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") handleSave();
                  if (e.key === "Escape") { setSaving(false); setNewName(""); }
                }}
                placeholder="Preset name..."
                style={{
                  flex: 1,
                  padding: "5px 8px",
                  borderRadius: 4,
                  border: "1px solid var(--color-border-default)",
                  background: "var(--color-bg-elevated)",
                  color: "var(--color-text-primary)",
                  fontSize: 12,
                  outline: "none",
                }}
              />
              <button
                onClick={handleSave}
                disabled={!newName.trim() || savePreset.isPending}
                style={{
                  padding: "5px 10px",
                  borderRadius: 4,
                  border: "none",
                  background: "var(--color-accent)",
                  color: "var(--color-accent-fg)",
                  fontSize: 12,
                  fontWeight: 500,
                  cursor: "pointer",
                  opacity: !newName.trim() ? 0.5 : 1,
                }}
              >
                Save
              </button>
            </div>
          ) : (
            <div
              onClick={() => setSaving(true)}
              style={{
                display: "flex",
                alignItems: "center",
                gap: 8,
                padding: "8px 14px",
                cursor: "pointer",
                fontSize: 13,
                color: "var(--color-accent)",
                transition: "background 100ms ease",
              }}
              onMouseEnter={(e) => { e.currentTarget.style.background = "var(--color-bg-elevated)"; }}
              onMouseLeave={(e) => { e.currentTarget.style.background = "transparent"; }}
            >
              <Plus size={13} /> Save current filters...
            </div>
          )}
        </div>
      )}
    </div>
  );
}

// Shows a compact summary of what's in a preset
function FilterSummary({ filters }: { filters: FilterPreset }) {
  const parts: string[] = [];
  if (filters.protocols.length) parts.push(filters.protocols.join("+"));
  if (filters.privacies.length) parts.push(filters.privacies.length + " privacy");
  if (filters.categories.length) parts.push(filters.categories.join("+"));
  if (filters.languages.length) parts.push(filters.languages.length + " lang");
  if (!parts.length) return null;
  return (
    <span style={{ fontSize: 10, color: "var(--color-text-muted)", whiteSpace: "nowrap", flexShrink: 0 }}>
      {parts.slice(0, 2).join(" · ")}
    </span>
  );
}
