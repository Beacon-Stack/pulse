import { useState, useMemo, useCallback } from "react";
import PresetDropdown from "./PresetDropdown";
import type { FilterPreset } from "./PresetDropdown";
import {
  Search,
  LayoutGrid,
  List,
  X,
  ChevronRight,
  Shield,
  ShieldAlert,
  Globe,
  Download,
  Newspaper,
  Check,
  AlertCircle,
  Loader2,
} from "lucide-react";
import { useCatalog, useCatalogLanguages } from "@/api/catalog";
import { useIndexers, useCreateIndexer } from "@/api/indexers";
import { toast } from "sonner";
import type { CatalogEntry, CatalogField } from "@/types";

// ── Filter constants ─────────────────────────────────────────────────────────

const PROTOCOLS = [
  { value: "torrent", label: "Torrent", icon: Download },
  { value: "usenet", label: "Usenet", icon: Newspaper },
] as const;

const PRIVACIES = [
  { value: "public", label: "Public", icon: Globe },
  { value: "semi-private", label: "Semi-Private", icon: Shield },
  { value: "private", label: "Private", icon: ShieldAlert },
] as const;

const CATEGORIES = ["Movies", "TV", "Audio", "Books", "XXX", "Other"] as const;

const categoryColors: Record<string, string> = {
  Movies: "#3b9eff",
  TV: "#34d399",
  Audio: "#f59e0b",
  Books: "#a78bfa",
  XXX: "#f87171",
  Other: "#6b7280",
};

// ── Language data ────────────────────────────────────────────────────────────

const LANG_DATA: Record<string, { flag: string; label: string }> = {
  "en-US": { flag: "\u{1F1FA}\u{1F1F8}", label: "English" },
  "en-AU": { flag: "\u{1F1E6}\u{1F1FA}", label: "English (AU)" },
  "en-GB": { flag: "\u{1F1EC}\u{1F1E7}", label: "English (UK)" },
  "fr-FR": { flag: "\u{1F1EB}\u{1F1F7}", label: "French" },
  "es-ES": { flag: "\u{1F1EA}\u{1F1F8}", label: "Spanish" },
  "de-DE": { flag: "\u{1F1E9}\u{1F1EA}", label: "German" },
  "it-IT": { flag: "\u{1F1EE}\u{1F1F9}", label: "Italian" },
  "pt-BR": { flag: "\u{1F1E7}\u{1F1F7}", label: "Portuguese" },
  "pt-PT": { flag: "\u{1F1F5}\u{1F1F9}", label: "Portuguese (PT)" },
  "ru-RU": { flag: "\u{1F1F7}\u{1F1FA}", label: "Russian" },
  "uk-UA": { flag: "\u{1F1FA}\u{1F1E6}", label: "Ukrainian" },
  "hu-HU": { flag: "\u{1F1ED}\u{1F1FA}", label: "Hungarian" },
  "ro-RO": { flag: "\u{1F1F7}\u{1F1F4}", label: "Romanian" },
  "pl-PL": { flag: "\u{1F1F5}\u{1F1F1}", label: "Polish" },
  "ja-JP": { flag: "\u{1F1EF}\u{1F1F5}", label: "Japanese" },
  "zh-CN": { flag: "\u{1F1E8}\u{1F1F3}", label: "Chinese" },
  "zh-TW": { flag: "\u{1F1F9}\u{1F1FC}", label: "Chinese (TW)" },
  "ko-KR": { flag: "\u{1F1F0}\u{1F1F7}", label: "Korean" },
  "da-DK": { flag: "\u{1F1E9}\u{1F1F0}", label: "Danish" },
  "sv-SE": { flag: "\u{1F1F8}\u{1F1EA}", label: "Swedish" },
  "nl-NL": { flag: "\u{1F1F3}\u{1F1F1}", label: "Dutch" },
  "fi-FI": { flag: "\u{1F1EB}\u{1F1EE}", label: "Finnish" },
  "cs-CZ": { flag: "\u{1F1E8}\u{1F1FF}", label: "Czech" },
  "tr-TR": { flag: "\u{1F1F9}\u{1F1F7}", label: "Turkish" },
  "ar-SA": { flag: "\u{1F1F8}\u{1F1E6}", label: "Arabic" },
  "he-IL": { flag: "\u{1F1EE}\u{1F1F1}", label: "Hebrew" },
  "bg-BG": { flag: "\u{1F1E7}\u{1F1EC}", label: "Bulgarian" },
  "th-TH": { flag: "\u{1F1F9}\u{1F1ED}", label: "Thai" },
  "vi-VN": { flag: "\u{1F1FB}\u{1F1F3}", label: "Vietnamese" },
  "sk-SK": { flag: "\u{1F1F8}\u{1F1F0}", label: "Slovak" },
  "hr-HR": { flag: "\u{1F1ED}\u{1F1F7}", label: "Croatian" },
  "el-GR": { flag: "\u{1F1EC}\u{1F1F7}", label: "Greek" },
  "fa-IR": { flag: "\u{1F1EE}\u{1F1F7}", label: "Persian" },
};

function langDisplay(code: string): { flag: string; label: string } {
  return LANG_DATA[code] ?? { flag: "\u{1F310}", label: code };
}

// ── Styles ───────────────────────────────────────────────────────────────────

const chipBase: React.CSSProperties = {
  display: "inline-flex",
  alignItems: "center",
  gap: 5,
  padding: "5px 12px",
  borderRadius: 6,
  fontSize: 12,
  fontWeight: 500,
  cursor: "pointer",
  border: "1px solid var(--color-border-default)",
  transition: "all 120ms ease",
  whiteSpace: "nowrap",
};

const chipActive: React.CSSProperties = {
  ...chipBase,
  background: "var(--color-accent-muted)",
  color: "var(--color-accent)",
  borderColor: "var(--color-accent)",
};

const chipInactive: React.CSSProperties = {
  ...chipBase,
  background: "transparent",
  color: "var(--color-text-secondary)",
};

// ── Main component ───────────────────────────────────────────────────────────

// Helper: toggle a value in a Set, returning a new Set.
function toggleSet<T>(set: Set<T>, value: T): Set<T> {
  const next = new Set(set);
  if (next.has(value)) next.delete(value);
  else next.add(value);
  return next;
}

export default function AddIndexerPage() {
  // Filter state — all multi-select via Sets
  const [searchQuery, setSearchQuery] = useState("");
  const [protocols, setProtocols] = useState<Set<string>>(new Set());
  const [privacies, setPrivacies] = useState<Set<string>>(new Set());
  const [categories, setCategories] = useState<Set<string>>(new Set());
  const [langs, setLangs] = useState<Set<string>>(new Set());
  const [langOpen, setLangOpen] = useState(false);
  const [viewMode, setViewMode] = useState<"grid" | "list">(() =>
    (localStorage.getItem("add-indexer-view") as "grid" | "list") || "grid"
  );
  const [activePresetName, setActivePresetName] = useState<string | null>(null);

  // Drawer state
  const [selectedEntry, setSelectedEntry] = useState<CatalogEntry | null>(null);

  // Data
  const { data: catalogData, isLoading, isError, refetch } = useCatalog();
  const { data: addedIndexers } = useIndexers();
  const { data: availableLangs } = useCatalogLanguages();

  const addedNames = useMemo(
    () => new Set((addedIndexers ?? []).map((i) => i.name.toLowerCase())),
    [addedIndexers]
  );

  // Client-side filtering — all filters are AND'd, values within a filter are OR'd
  const filtered = useMemo(() => {
    if (!catalogData?.entries) return [];
    let entries = catalogData.entries;

    if (protocols.size > 0) entries = entries.filter((e) => protocols.has(e.protocol));
    if (privacies.size > 0) entries = entries.filter((e) => privacies.has(e.privacy));
    if (categories.size > 0) entries = entries.filter((e) => e.categories.some((c) => categories.has(c)));
    if (langs.size > 0) entries = entries.filter((e) => langs.has(e.language));
    if (searchQuery) {
      const q = searchQuery.toLowerCase();
      entries = entries.filter(
        (e) =>
          e.name.toLowerCase().includes(q) ||
          e.description.toLowerCase().includes(q) ||
          e.categories.some((c) => c.toLowerCase().includes(q))
      );
    }

    return [...entries].sort((a, b) => {
      const aAdded = addedNames.has(a.name.toLowerCase());
      const bAdded = addedNames.has(b.name.toLowerCase());
      if (aAdded !== bAdded) return aAdded ? 1 : -1;
      return a.name.localeCompare(b.name);
    });
  }, [catalogData, protocols, privacies, categories, langs, searchQuery, addedNames]);

  const activeFilterCount = protocols.size + privacies.size + categories.size + langs.size;

  const toggleView = useCallback((mode: "grid" | "list") => {
    setViewMode(mode);
    localStorage.setItem("add-indexer-view", mode);
  }, []);

  const clearFilters = useCallback(() => {
    setSearchQuery("");
    setProtocols(new Set());
    setPrivacies(new Set());
    setCategories(new Set());
    setLangs(new Set());
    setLangOpen(false);
    setActivePresetName(null);
  }, []);

  // Preset helpers
  const getCurrentFilters = useCallback((): FilterPreset => ({
    protocols: [...protocols],
    privacies: [...privacies],
    categories: [...categories],
    languages: [...langs],
    search: searchQuery,
  }), [protocols, privacies, categories, langs, searchQuery]);

  const applyPreset = useCallback((preset: FilterPreset) => {
    setProtocols(new Set(preset.protocols));
    setPrivacies(new Set(preset.privacies));
    setCategories(new Set(preset.categories));
    setLangs(new Set(preset.languages));
    setSearchQuery(preset.search || "");
    setLangOpen(preset.languages.length > 0);
    // We don't set activePresetName here — PresetDropdown handles the name via onApply
  }, []);


  return (
    <div style={{ padding: 24, maxWidth: 1400 }}>
      {/* Header */}
      <div style={{ marginBottom: 20 }}>
        <h1 style={{ margin: 0, fontSize: 20, fontWeight: 600, color: "var(--color-text-primary)" }}>
          Add Indexer
        </h1>
        <p style={{ margin: "4px 0 0", fontSize: 13, color: "var(--color-text-secondary)" }}>
          Browse and configure indexers from the catalog.
          {catalogData && (
            <span style={{ color: "var(--color-text-muted)", marginLeft: 6 }}>
              {filtered.length} of {catalogData.total} indexers
            </span>
          )}
        </p>
      </div>

      {/* Search + view toggle */}
      <div style={{ display: "flex", gap: 8, marginBottom: 12, alignItems: "center" }}>
        <div style={{ position: "relative", flex: 1, maxWidth: 480 }}>
          <Search
            size={16}
            style={{
              position: "absolute",
              left: 12,
              top: "50%",
              transform: "translateY(-50%)",
              color: "var(--color-text-muted)",
              pointerEvents: "none",
            }}
          />
          <input
            type="text"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder="Search indexers by name, description, or category..."
            style={{
              width: "100%",
              padding: "9px 12px 9px 36px",
              borderRadius: 8,
              border: "1px solid var(--color-border-default)",
              background: "var(--color-bg-elevated)",
              color: "var(--color-text-primary)",
              fontSize: 13,
              outline: "none",
            }}
          />
          {searchQuery && (
            <button
              onClick={() => setSearchQuery("")}
              style={{
                position: "absolute",
                right: 8,
                top: "50%",
                transform: "translateY(-50%)",
                background: "none",
                border: "none",
                cursor: "pointer",
                color: "var(--color-text-muted)",
                padding: 2,
              }}
            >
              <X size={14} />
            </button>
          )}
        </div>

        <PresetDropdown
          currentFilters={getCurrentFilters}
          onApply={(preset, name) => {
            applyPreset(preset);
            setActivePresetName(name);
          }}
          activePresetName={activePresetName}
        />

        <div style={{ display: "flex", gap: 2, background: "var(--color-bg-elevated)", borderRadius: 6, padding: 2, border: "1px solid var(--color-border-subtle)" }}>
          <button onClick={() => toggleView("grid")} title="Grid view" style={{ display: "flex", padding: 6, borderRadius: 4, border: "none", cursor: "pointer", background: viewMode === "grid" ? "var(--color-accent-muted)" : "transparent", color: viewMode === "grid" ? "var(--color-accent)" : "var(--color-text-muted)" }}>
            <LayoutGrid size={16} />
          </button>
          <button onClick={() => toggleView("list")} title="List view" style={{ display: "flex", padding: 6, borderRadius: 4, border: "none", cursor: "pointer", background: viewMode === "list" ? "var(--color-accent-muted)" : "transparent", color: viewMode === "list" ? "var(--color-accent)" : "var(--color-text-muted)" }}>
            <List size={16} />
          </button>
        </div>
      </div>

      {/* Filter chips — all multi-select */}
      <div style={{ display: "flex", flexWrap: "wrap", gap: 6, marginBottom: 20, alignItems: "center" }}>
        {/* Protocol */}
        {PROTOCOLS.map((p) => {
          const Icon = p.icon;
          const active = protocols.has(p.value);
          return (
            <button key={p.value} onClick={() => setProtocols(toggleSet(protocols, p.value))} style={active ? chipActive : chipInactive}>
              <Icon size={13} /> {p.label}
            </button>
          );
        })}

        <div style={{ width: 1, height: 24, background: "var(--color-border-subtle)", margin: "0 4px" }} />

        {/* Privacy */}
        {PRIVACIES.map((p) => {
          const Icon = p.icon;
          const active = privacies.has(p.value);
          return (
            <button key={p.value} onClick={() => setPrivacies(toggleSet(privacies, p.value))} style={active ? chipActive : chipInactive}>
              <Icon size={13} /> {p.label}
            </button>
          );
        })}

        <div style={{ width: 1, height: 24, background: "var(--color-border-subtle)", margin: "0 4px" }} />

        {/* Categories */}
        {CATEGORIES.map((c) => {
          const active = categories.has(c);
          return (
            <button key={c} onClick={() => setCategories(toggleSet(categories, c))} style={active ? chipActive : chipInactive}>
              {c}
            </button>
          );
        })}

        <div style={{ width: 1, height: 24, background: "var(--color-border-subtle)", margin: "0 4px" }} />

        {/* Language — toggle button that opens/closes the language chip row */}
        {availableLangs && availableLangs.length > 1 && (
          <button
            onClick={() => setLangOpen(!langOpen)}
            style={{
              ...langs.size > 0 ? chipActive : chipInactive,
              gap: 4,
            }}
          >
            {langs.size > 0
              ? `${[...langs].map((l) => langDisplay(l).flag).join("")} ${langs.size} language${langs.size > 1 ? "s" : ""}`
              : "Language"}
            <span style={{ fontSize: 9, marginLeft: 2 }}>{langOpen ? "▴" : "▾"}</span>
          </button>
        )}

        {activeFilterCount > 0 && (
          <button onClick={clearFilters} style={{ ...chipBase, color: "var(--color-text-muted)", borderStyle: "dashed" }}>
            <X size={12} /> Clear all
          </button>
        )}
      </div>

      {/* Language chips row — shown when language toggle is open */}
      {langOpen && availableLangs && (
        <div style={{
          display: "flex",
          flexWrap: "wrap",
          gap: 5,
          marginBottom: 16,
          padding: "10px 12px",
          borderRadius: 8,
          background: "var(--color-bg-surface)",
          border: "1px solid var(--color-border-subtle)",
        }}>
          {[...availableLangs]
            .sort((a, b) => langDisplay(a).label.localeCompare(langDisplay(b).label))
            .map((lang) => {
              const d = langDisplay(lang);
              const active = langs.has(lang);
              return (
                <button
                  key={lang}
                  onClick={() => setLangs(toggleSet(langs, lang))}
                  style={{
                    ...active ? chipActive : chipInactive,
                    fontSize: 11,
                    padding: "3px 10px",
                    gap: 4,
                  }}
                >
                  <span>{d.flag}</span> {d.label}
                </button>
              );
            })}
        </div>
      )}

      {/* Content */}
      {isLoading ? (
        <div style={{ padding: 80, textAlign: "center", color: "var(--color-text-muted)", fontSize: 13 }}>
          <Loader2 size={24} style={{ animation: "spin 1s linear infinite", marginBottom: 8 }} />
          <div>Loading catalog...</div>
        </div>
      ) : isError ? (
        <div style={{ padding: 80, textAlign: "center" }}>
          <AlertCircle size={24} style={{ color: "var(--color-danger)", marginBottom: 8 }} />
          <div style={{ color: "var(--color-text-secondary)", fontSize: 13, marginBottom: 12 }}>Failed to load the indexer catalog.</div>
          <button onClick={() => refetch()} style={{ padding: "6px 14px", borderRadius: 6, border: "1px solid var(--color-border-default)", background: "var(--color-bg-elevated)", color: "var(--color-text-secondary)", fontSize: 13, cursor: "pointer" }}>Retry</button>
        </div>
      ) : filtered.length === 0 ? (
        <div style={{ padding: 80, textAlign: "center" }}>
          <Search size={24} style={{ color: "var(--color-text-muted)", marginBottom: 8 }} />
          <div style={{ color: "var(--color-text-secondary)", fontSize: 14, fontWeight: 500, marginBottom: 4 }}>No indexers match your filters</div>
          <div style={{ color: "var(--color-text-muted)", fontSize: 13, marginBottom: 12 }}>Try broadening your search or clearing some filters.</div>
          <button onClick={clearFilters} style={{ padding: "6px 14px", borderRadius: 6, border: "1px solid var(--color-border-default)", background: "var(--color-bg-elevated)", color: "var(--color-text-secondary)", fontSize: 13, cursor: "pointer" }}>Clear filters</button>
        </div>
      ) : viewMode === "grid" ? (
        <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(300px, 1fr))", gap: 10 }}>
          {filtered.map((entry) => (
            <GridCard key={entry.id} entry={entry} isAdded={addedNames.has(entry.name.toLowerCase())} onSelect={() => setSelectedEntry(entry)} />
          ))}
        </div>
      ) : (
        <div style={{ display: "flex", flexDirection: "column", gap: 2 }}>
          {filtered.map((entry) => (
            <ListRow key={entry.id} entry={entry} isAdded={addedNames.has(entry.name.toLowerCase())} onSelect={() => setSelectedEntry(entry)} />
          ))}
        </div>
      )}

      {/* Config drawer */}
      {selectedEntry && (
        <ConfigDrawer
          entry={selectedEntry}
          isAdded={addedNames.has(selectedEntry.name.toLowerCase())}
          onClose={() => setSelectedEntry(null)}
        />
      )}
    </div>
  );
}

// ── Grid Card ────────────────────────────────────────────────────────────────

function GridCard({ entry, isAdded, onSelect }: { entry: CatalogEntry; isAdded: boolean; onSelect: () => void }) {
  return (
    <button
      onClick={onSelect}
      style={{
        display: "flex",
        flexDirection: "column",
        padding: 16,
        borderRadius: 8,
        border: "1px solid var(--color-border-subtle)",
        background: isAdded ? "var(--color-bg-base)" : "var(--color-bg-surface)",
        cursor: "pointer",
        textAlign: "left",
        transition: "border-color 120ms ease, box-shadow 120ms ease",
        opacity: isAdded ? 0.6 : 1,
        position: "relative",
        minHeight: 140,
      }}
      onMouseEnter={(e) => {
        if (!isAdded) {
          e.currentTarget.style.borderColor = "var(--color-accent)";
          e.currentTarget.style.boxShadow = "0 0 0 1px var(--color-accent)";
        }
      }}
      onMouseLeave={(e) => {
        e.currentTarget.style.borderColor = "var(--color-border-subtle)";
        e.currentTarget.style.boxShadow = "none";
      }}
    >
      {/* Top: name + badges */}
      <div style={{ display: "flex", alignItems: "center", gap: 6, marginBottom: 6 }}>
        <span style={{ fontSize: 14, fontWeight: 600, color: "var(--color-text-primary)", flex: 1, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
          {entry.name}
        </span>
        {isAdded && (
          <span style={{ display: "inline-flex", alignItems: "center", gap: 3, fontSize: 10, fontWeight: 600, color: "var(--color-success)", background: "color-mix(in srgb, var(--color-success) 12%, transparent)", padding: "2px 6px", borderRadius: 3 }}>
            <Check size={10} /> Added
          </span>
        )}
      </div>

      {/* Protocol + privacy badges */}
      <div style={{ display: "flex", gap: 4, marginBottom: 8 }}>
        <ProtocolBadge protocol={entry.protocol} />
        <PrivacyBadge privacy={entry.privacy} />
        {entry.language !== "en-US" && (
          <span style={{ fontSize: 10, padding: "2px 6px", borderRadius: 3, background: "var(--color-bg-subtle)", color: "var(--color-text-muted)" }} title={langDisplay(entry.language).label}>
            {langDisplay(entry.language).flag} {langDisplay(entry.language).label}
          </span>
        )}
      </div>

      {/* Description */}
      <p style={{
        margin: 0,
        fontSize: 12,
        color: "var(--color-text-muted)",
        lineHeight: 1.4,
        flex: 1,
        display: "-webkit-box",
        WebkitLineClamp: 2,
        WebkitBoxOrient: "vertical",
        overflow: "hidden",
      }}>
        {entry.description}
      </p>

      {/* Categories */}
      <div style={{ display: "flex", flexWrap: "wrap", gap: 3, marginTop: 10 }}>
        {entry.categories.map((cat) => (
          <span
            key={cat}
            style={{
              fontSize: 10,
              fontWeight: 500,
              padding: "1px 6px",
              borderRadius: 3,
              color: categoryColors[cat] ?? "#6b7280",
              background: `color-mix(in srgb, ${categoryColors[cat] ?? "#6b7280"} 10%, transparent)`,
            }}
          >
            {cat}
          </span>
        ))}
      </div>
    </button>
  );
}

// ── List Row ─────────────────────────────────────────────────────────────────

function ListRow({ entry, isAdded, onSelect }: { entry: CatalogEntry; isAdded: boolean; onSelect: () => void }) {
  return (
    <button
      onClick={onSelect}
      style={{
        display: "flex",
        alignItems: "center",
        gap: 12,
        padding: "10px 16px",
        borderRadius: 6,
        border: "1px solid transparent",
        background: isAdded ? "var(--color-bg-base)" : "transparent",
        cursor: "pointer",
        textAlign: "left",
        transition: "background 100ms ease",
        opacity: isAdded ? 0.6 : 1,
        width: "100%",
      }}
      onMouseEnter={(e) => { e.currentTarget.style.background = "var(--color-bg-surface)"; }}
      onMouseLeave={(e) => { e.currentTarget.style.background = isAdded ? "var(--color-bg-base)" : "transparent"; }}
    >
      <span style={{ fontSize: 14, fontWeight: 500, color: "var(--color-text-primary)", width: 180, flexShrink: 0, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
        {entry.name}
      </span>
      <ProtocolBadge protocol={entry.protocol} />
      <PrivacyBadge privacy={entry.privacy} />
      {entry.categories.map((cat) => (
        <span key={cat} style={{ fontSize: 10, fontWeight: 500, padding: "1px 6px", borderRadius: 3, color: categoryColors[cat] ?? "#6b7280", background: `color-mix(in srgb, ${categoryColors[cat] ?? "#6b7280"} 10%, transparent)` }}>
          {cat}
        </span>
      ))}
      <span style={{ flex: 1, fontSize: 12, color: "var(--color-text-muted)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", marginLeft: 8 }}>
        {entry.description}
      </span>
      {isAdded ? (
        <span style={{ display: "inline-flex", alignItems: "center", gap: 3, fontSize: 10, fontWeight: 600, color: "var(--color-success)", flexShrink: 0 }}>
          <Check size={10} /> Added
        </span>
      ) : (
        <ChevronRight size={16} style={{ color: "var(--color-text-muted)", flexShrink: 0 }} />
      )}
    </button>
  );
}

// ── Config Drawer ────────────────────────────────────────────────────────────

function ConfigDrawer({
  entry,
  isAdded,
  onClose,
}: {
  entry: CatalogEntry;
  isAdded: boolean;
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
                You can add another instance with a different configuration if needed.
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

// ── Field Input ──────────────────────────────────────────────────────────────

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

// ── Shared badges ────────────────────────────────────────────────────────────

function ProtocolBadge({ protocol }: { protocol: string }) {
  const isTorrent = protocol === "torrent";
  return (
    <span style={{
      display: "inline-flex",
      alignItems: "center",
      gap: 3,
      fontSize: 10,
      fontWeight: 600,
      padding: "2px 6px",
      borderRadius: 3,
      color: isTorrent ? "#3b9eff" : "#f59e0b",
      background: isTorrent
        ? "color-mix(in srgb, #3b9eff 12%, transparent)"
        : "color-mix(in srgb, #f59e0b 12%, transparent)",
      textTransform: "uppercase",
      letterSpacing: "0.04em",
    }}>
      {isTorrent ? <Download size={9} /> : <Newspaper size={9} />}
      {protocol}
    </span>
  );
}

function PrivacyBadge({ privacy }: { privacy: string }) {
  const colors: Record<string, string> = {
    public: "var(--color-success)",
    "semi-private": "var(--color-warning)",
    private: "var(--color-danger)",
  };
  const color = colors[privacy] ?? "var(--color-text-muted)";
  return (
    <span style={{
      fontSize: 10,
      fontWeight: 500,
      padding: "2px 6px",
      borderRadius: 3,
      color,
      background: `color-mix(in srgb, ${color} 10%, transparent)`,
    }}>
      {privacy}
    </span>
  );
}
