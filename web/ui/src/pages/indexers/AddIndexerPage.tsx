import { useState, useMemo, useCallback } from "react";
import { Link } from "react-router-dom";
import PresetDropdown from "./PresetDropdown";
import type { FilterPreset } from "./PresetDropdown";
import {
  Search,
  LayoutGrid,
  List,
  X,
  Shield,
  ShieldAlert,
  Globe,
  Download,
  Newspaper,
  Check,
  AlertCircle,
  Loader2,
  ListChecks,
} from "lucide-react";
import { useCatalog, useCatalogLanguages, useCatalogHealth } from "@/api/catalog";
import { langDisplay } from "@/lib/languages";
import { useAddIndexerDefaults } from "@/api/ui-settings";
import { needsOverlayNetwork } from "@/lib/catalogHealth";
import { useIndexers, useIndexerStats } from "@/api/indexers";
import type { CatalogEntry, IndexerStats } from "@/types";
import ConfigDrawer, { TesterStatusLine } from "./IndexerConfigDrawer";
import { GridCard, ListRow } from "./IndexerCatalogCards";

// Filter constants

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

type SortMode = "recommended" | "fastest" | "results" | "seeders" | "name";

const SORT_MODES: { value: SortMode; label: string; help: string }[] = [
  { value: "recommended", label: "Recommended", help: "Best overall, from background test searches" },
  { value: "fastest", label: "Fastest", help: "Quickest typical search time first" },
  { value: "results", label: "Most results", help: "Most results per test search first" },
  { value: "seeders", label: "Most seeders", help: "Best-seeded results first (typical seeders on the top results)" },
  { value: "name", label: "A–Z", help: "Alphabetical" },
];

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

// Main component

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
  // Sort order. All but A–Z use the background tester's results.
  const [sortMode, setSortMode] = useState<SortMode>(() => {
    try {
      const stored = localStorage.getItem("add-indexer-sort");
      return SORT_MODES.some((m) => m.value === stored) ? (stored as SortMode) : "recommended";
    } catch {
      return "recommended";
    }
  });
  const [worksOnly, setWorksOnly] = useState(false);
  const [addedOnly, setAddedOnly] = useState(false);

  // Drawer state
  const [selectedEntry, setSelectedEntry] = useState<CatalogEntry | null>(null);

  // Data
  const { data: catalogData, isLoading, isError, refetch } = useCatalog();
  const { data: addedIndexers } = useIndexers();
  const { data: availableLangs } = useCatalogLanguages();
  const { data: healthData } = useCatalogHealth();
  const { data: defaults } = useAddIndexerDefaults();
  const hasDefaults =
    !!defaults &&
    defaults.protocols.length + defaults.privacies.length + defaults.categories.length + defaults.languages.length > 0;

  // Start from the defaults set in App Settings, once, when they load
  // (state adjusted during render, React's pattern for this).
  const [defaultsApplied, setDefaultsApplied] = useState(false);
  if (defaults && !defaultsApplied) {
    setDefaultsApplied(true);
    setProtocols(new Set(defaults.protocols));
    setPrivacies(new Set(defaults.privacies));
    setCategories(new Set(defaults.categories));
    setLangs(new Set(defaults.languages));
  }

  const resetToDefaults = useCallback(() => {
    if (!defaults) return;
    setProtocols(new Set(defaults.protocols));
    setPrivacies(new Set(defaults.privacies));
    setCategories(new Set(defaults.categories));
    setLangs(new Set(defaults.languages));
    setActivePresetName(null);
  }, [defaults]);

  const healthById = useMemo(
    () => new Map((healthData?.sites ?? []).map((h) => [h.catalog_id, h])),
    [healthData]
  );
  const eligible = useMemo(() => new Set(healthData?.eligible ?? []), [healthData]);

  const addedNames = useMemo(
    () => new Set((addedIndexers ?? []).map((i) => i.name.toLowerCase())),
    [addedIndexers]
  );
  const addedCatalogIds = useMemo(() => new Set(healthData?.added ?? []), [healthData]);
  const isAdded = useCallback(
    (e: CatalogEntry) => addedNames.has(e.name.toLowerCase()) || addedCatalogIds.has(e.id),
    [addedNames, addedCatalogIds]
  );
  // Added indexers' own record, for their real-search stats and settings link.
  const { data: statsData } = useIndexerStats();
  const addedIndexerFor = useCallback(
    (e: CatalogEntry) => (addedIndexers ?? []).find((i) => i.name.toLowerCase() === e.name.toLowerCase()),
    [addedIndexers]
  );
  const realStatsFor = useCallback(
    (e: CatalogEntry): IndexerStats | undefined => {
      const idx = addedIndexerFor(e);
      return idx ? statsData?.indexers.find((s) => s.indexer_id === idx.id) : undefined;
    },
    [addedIndexerFor, statsData]
  );
  const addedTotal = useMemo(() => (catalogData?.entries ?? []).filter(isAdded).length, [catalogData, isAdded]);

  // Client-side filtering — all filters are AND'd, values within a filter are OR'd
  const filtered = useMemo(() => {
    if (!catalogData?.entries) return [];
    let entries = catalogData.entries;

    if (protocols.size > 0) entries = entries.filter((e) => protocols.has(e.protocol));
    if (privacies.size > 0) entries = entries.filter((e) => privacies.has(e.privacy));
    if (categories.size > 0) entries = entries.filter((e) => e.categories.some((c) => categories.has(c)));
    if (langs.size > 0) entries = entries.filter((e) => langs.has(e.language));
    if (worksOnly) entries = entries.filter((e) => healthById.get(e.id)?.verdict === "works");
    if (addedOnly) entries = entries.filter(isAdded);
    if (searchQuery) {
      const q = searchQuery.toLowerCase();
      entries = entries.filter(
        (e) =>
          e.name.toLowerCase().includes(q) ||
          e.description.toLowerCase().includes(q) ||
          e.categories.some((c) => c.toLowerCase().includes(q))
      );
    }

    // Recommended: tested sites by score, then ones waiting to be tested,
    // then the rest (private, other categories); added ones always last.
    const rank = (e: CatalogEntry) => {
      if (needsOverlayNetwork(e.urls)) return -2;
      const h = healthById.get(e.id);
      if (h) return h.score;
      return eligible.has(e.id) ? 0.5 : -1;
    };
    // Fastest / most results / most seeders: working sites by that
    // measurement (higher is better), then the rest in recommended order.
    const metric = (e: CatalogEntry): number | null => {
      const h = healthById.get(e.id);
      if (!h || h.verdict !== "works") return null;
      switch (sortMode) {
        case "fastest":
          return -h.median_ms;
        case "results":
          return h.avg_results;
        case "seeders":
          return h.median_seeders;
        default:
          return null;
      }
    };
    return [...entries].sort((a, b) => {
      const aAdded = isAdded(a);
      const bAdded = isAdded(b);
      if (aAdded !== bAdded) return aAdded ? 1 : -1;
      if (sortMode !== "name") {
        const ma = metric(a);
        const mb = metric(b);
        if (ma !== null || mb !== null) {
          if (ma === null) return 1;
          if (mb === null) return -1;
          if (ma !== mb) return mb - ma;
        }
        const diff = rank(b) - rank(a);
        if (diff !== 0) return diff;
      }
      return a.name.localeCompare(b.name);
    });
  }, [catalogData, protocols, privacies, categories, langs, searchQuery, isAdded, worksOnly, addedOnly, healthById, eligible, sortMode]);

  const changeSort = useCallback((mode: SortMode) => {
    setSortMode(mode);
    try {
      localStorage.setItem("add-indexer-sort", mode);
    } catch {
      // private mode: the choice just isn't remembered
    }
  }, []);

  const activeFilterCount =
    protocols.size + privacies.size + categories.size + langs.size + (worksOnly ? 1 : 0) + (addedOnly ? 1 : 0);
  // Added indexers the other filters are hiding while "Added" is on.
  const addedHidden = addedOnly ? addedTotal - filtered.length : 0;

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
    setWorksOnly(false);
    setAddedOnly(false);
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
        {healthData && <TesterStatusLine data={healthData} />}
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

        <label style={{ display: "inline-flex", alignItems: "center", gap: 6, fontSize: 12, color: "var(--color-text-muted)" }}>
          Sort
          <select
            value={sortMode}
            onChange={(e) => changeSort(e.target.value as SortMode)}
            title={SORT_MODES.find((m) => m.value === sortMode)?.help}
            style={{ padding: "6px 8px", borderRadius: 6, border: "1px solid var(--color-border-default)", background: "var(--color-bg-elevated)", color: "var(--color-text-primary)", fontSize: 12, cursor: "pointer" }}
          >
            {SORT_MODES.map((m) => (
              <option key={m.value} value={m.value}>
                {m.label}
              </option>
            ))}
          </select>
        </label>

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

        <button
          onClick={() => setWorksOnly(!worksOnly)}
          title="Only indexers whose last background test search found results"
          style={worksOnly ? chipActive : chipInactive}
        >
          <Check size={13} /> Works
        </button>

        {addedTotal > 0 && (
          <button
            onClick={() => setAddedOnly(!addedOnly)}
            title="Only the indexers you've already added"
            style={addedOnly ? chipActive : chipInactive}
          >
            <ListChecks size={13} /> Added ({addedTotal})
          </button>
        )}

        {activeFilterCount > 0 && (
          <button onClick={clearFilters} style={{ ...chipBase, color: "var(--color-text-muted)", borderStyle: "dashed" }}>
            <X size={12} /> Clear all
          </button>
        )}

        {hasDefaults && (
          <span style={{ fontSize: 12, color: "var(--color-text-muted)", marginLeft: 4 }}>
            <button
              onClick={resetToDefaults}
              style={{ background: "none", border: "none", padding: 0, font: "inherit", color: "var(--color-accent)", cursor: "pointer" }}
            >
              Reset to defaults
            </button>
            {" · "}
            <Link to="/settings/app" style={{ color: "var(--color-text-muted)" }}>
              Change defaults
            </Link>
          </span>
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

      {addedHidden > 0 && (
        <div style={{ marginBottom: 12, fontSize: 12, color: "var(--color-text-secondary)" }}>
          {addedHidden} more added indexer{addedHidden === 1 ? " is" : "s are"} hidden by your other filters.{" "}
          <button
            onClick={() => {
              clearFilters();
              setAddedOnly(true);
            }}
            style={{ background: "none", border: "none", padding: 0, font: "inherit", color: "var(--color-accent)", cursor: "pointer" }}
          >
            Show all {addedTotal}
          </button>
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
            <GridCard key={entry.id} entry={entry} isAdded={isAdded(entry)} dim={isAdded(entry) && !addedOnly} realStats={realStatsFor(entry)} health={healthById.get(entry.id)} untested={eligible.has(entry.id)} onSelect={() => setSelectedEntry(entry)} />
          ))}
        </div>
      ) : (
        <div style={{ display: "flex", flexDirection: "column", gap: 2 }}>
          {filtered.map((entry) => (
            <ListRow key={entry.id} entry={entry} isAdded={isAdded(entry)} dim={isAdded(entry) && !addedOnly} realStats={realStatsFor(entry)} health={healthById.get(entry.id)} untested={eligible.has(entry.id)} onSelect={() => setSelectedEntry(entry)} />
          ))}
        </div>
      )}

      {/* Config drawer */}
      {selectedEntry && (
        <ConfigDrawer
          entry={selectedEntry}
          isAdded={isAdded(selectedEntry)}
          addedIndexerId={addedIndexerFor(selectedEntry)?.id}
          health={healthById.get(selectedEntry.id)}
          onClose={() => setSelectedEntry(null)}
        />
      )}
    </div>
  );
}
