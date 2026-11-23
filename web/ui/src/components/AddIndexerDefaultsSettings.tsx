import { useState } from "react";
import { Check, Loader2 } from "lucide-react";
import { sectionHeader } from "@/lib/styles";
import { langDisplay } from "@/lib/languages";
import { useCatalogLanguages } from "@/api/catalog";
import { useAddIndexerDefaults, useSaveAddIndexerDefaults, type AddIndexerDefaults } from "@/api/ui-settings";

// App Settings section: the filters the Add Indexer page starts with.

const GROUPS: { key: keyof Omit<AddIndexerDefaults, "languages">; label: string; options: { value: string; label: string }[] }[] = [
  { key: "protocols", label: "Type", options: [{ value: "torrent", label: "Torrent" }, { value: "usenet", label: "Usenet" }] },
  {
    key: "privacies",
    label: "Access",
    options: [
      { value: "public", label: "Public" },
      { value: "semi-private", label: "Semi-private" },
      { value: "private", label: "Private" },
    ],
  },
  {
    key: "categories",
    label: "Categories",
    options: ["Movies", "TV", "Audio", "Books", "XXX", "Other"].map((c) => ({ value: c, label: c })),
  },
];

const empty: AddIndexerDefaults = { protocols: [], privacies: [], categories: [], languages: [] };

const chip = (active: boolean): React.CSSProperties => ({
  display: "inline-flex",
  alignItems: "center",
  gap: 4,
  padding: "5px 12px",
  borderRadius: 6,
  fontSize: 12,
  fontWeight: 500,
  cursor: "pointer",
  whiteSpace: "nowrap",
  border: `1px solid ${active ? "var(--color-accent)" : "var(--color-border-default)"}`,
  background: active ? "var(--color-accent-muted)" : "transparent",
  color: active ? "var(--color-accent)" : "var(--color-text-secondary)",
});

const sameSet = (a: string[], b: string[]) => a.length === b.length && a.every((x) => b.includes(x));

export default function AddIndexerDefaultsSettings() {
  const { data: saved, isLoading } = useAddIndexerDefaults();
  const { data: languages } = useCatalogLanguages();
  const save = useSaveAddIndexerDefaults();
  // Unsaved edits; null means "showing what's saved".
  const [edits, setEdits] = useState<AddIndexerDefaults | null>(null);
  const current = saved ?? empty;
  const draft = edits ?? current;

  const toggle = (key: keyof AddIndexerDefaults, value: string) =>
    setEdits({
      ...draft,
      [key]: draft[key].includes(value) ? draft[key].filter((v) => v !== value) : [...draft[key], value],
    });

  const changed = (Object.keys(empty) as (keyof AddIndexerDefaults)[]).some((k) => !sameSet(draft[k], current[k]));

  const row = (label: string, children: React.ReactNode) => (
    <div style={{ display: "flex", gap: 12, alignItems: "flex-start", flexWrap: "wrap", marginBottom: 12 }}>
      <div style={{ width: 90, flexShrink: 0, fontSize: 12, color: "var(--color-text-muted)", paddingTop: 6 }}>{label}</div>
      <div style={{ display: "flex", flexWrap: "wrap", gap: 6, flex: 1, minWidth: 0 }}>{children}</div>
    </div>
  );

  const sortedLangs = [...(languages ?? [])].sort((a, b) => langDisplay(a).label.localeCompare(langDisplay(b).label));

  return (
    <div>
      <h3 style={sectionHeader}>Add Indexer defaults</h3>
      <p style={{ margin: "0 0 16px", fontSize: 13, color: "var(--color-text-secondary)", lineHeight: 1.5 }}>
        Filters the Add Indexer page starts with. Leave a row empty to show everything. You can still change the
        filters on the page; this only sets where it starts.
      </p>

      {isLoading ? (
        <div style={{ fontSize: 13, color: "var(--color-text-muted)" }}>Loading...</div>
      ) : (
        <>
          {GROUPS.map((g) =>
            row(
              g.label,
              g.options.map((o) => {
                const active = draft[g.key].includes(o.value);
                return (
                  <button key={o.value} type="button" onClick={() => toggle(g.key, o.value)} style={chip(active)} aria-pressed={active}>
                    {active && <Check size={12} />} {o.label}
                  </button>
                );
              }),
            ),
          )}
          {row(
            "Language",
            sortedLangs.length === 0 ? (
              <span style={{ fontSize: 12, color: "var(--color-text-muted)", paddingTop: 6 }}>Loading languages...</span>
            ) : (
              sortedLangs.map((code) => {
                const active = draft.languages.includes(code);
                const d = langDisplay(code);
                return (
                  <button key={code} type="button" onClick={() => toggle("languages", code)} style={{ ...chip(active), fontSize: 11, padding: "3px 10px" }} aria-pressed={active}>
                    <span>{d.flag}</span> {d.label}
                  </button>
                );
              })
            ),
          )}

          <div style={{ display: "flex", gap: 8, marginTop: 16 }}>
            <button
              type="button"
              disabled={!changed || save.isPending}
              onClick={() => save.mutate(draft, { onSuccess: () => setEdits(null) })}
              style={{
                display: "inline-flex",
                alignItems: "center",
                gap: 6,
                padding: "7px 16px",
                borderRadius: 6,
                border: "none",
                background: changed ? "var(--color-accent)" : "var(--color-bg-subtle)",
                color: changed ? "var(--color-accent-fg)" : "var(--color-text-muted)",
                fontSize: 13,
                fontWeight: 500,
                cursor: changed ? "pointer" : "default",
              }}
            >
              {save.isPending && <Loader2 size={13} style={{ animation: "spin 1s linear infinite" }} />}
              Save defaults
            </button>
            {changed && (
              <button
                type="button"
                onClick={() => setEdits(null)}
                style={{ padding: "7px 14px", borderRadius: 6, border: "1px solid var(--color-border-default)", background: "transparent", color: "var(--color-text-secondary)", fontSize: 13, cursor: "pointer" }}
              >
                Undo changes
              </button>
            )}
          </div>
        </>
      )}
    </div>
  );
}
