import { useState, useEffect } from "react";
import PageHeader from "@/components/PageHeader";
import { card } from "@/lib/styles";
import {
  useSharedSettings,
  useUpdateSharedSettings,
  type ColonReplacement,
} from "@/api/shared-settings";

const colonOptions: { value: ColonReplacement; label: string; hint: string }[] = [
  { value: "space-dash", label: "Space-dash", hint: "Convert ': ' to ' - '. Default." },
  { value: "dash", label: "Dash", hint: "Convert ':' to '-'." },
  { value: "delete", label: "Delete", hint: "Remove ':' entirely." },
  { value: "smart", label: "Smart", hint: "Keep if surrounding chars are safe." },
];

export default function SharedSettingsPage() {
  const { data, isLoading } = useSharedSettings();
  const update = useUpdateSharedSettings();

  const [colonReplacement, setColonReplacement] = useState<ColonReplacement>("space-dash");
  const [importExtraFiles, setImportExtraFiles] = useState(false);
  const [extraFileExtensions, setExtraFileExtensions] = useState("srt,nfo");
  const [renameFiles, setRenameFiles] = useState(true);

  useEffect(() => {
    if (!data) return;
    setColonReplacement(data.colon_replacement);
    setImportExtraFiles(data.import_extra_files);
    setExtraFileExtensions(data.extra_file_extensions);
    setRenameFiles(data.rename_files);
  }, [data]);

  function handleSave() {
    update.mutate({
      colon_replacement: colonReplacement,
      import_extra_files: importExtraFiles,
      extra_file_extensions: extraFileExtensions,
      rename_files: renameFiles,
    });
  }

  const label: React.CSSProperties = {
    display: "block",
    fontSize: 13,
    fontWeight: 600,
    color: "var(--color-text-primary)",
    marginBottom: 4,
  };
  const hint: React.CSSProperties = {
    fontSize: 12,
    color: "var(--color-text-muted)",
    marginTop: 4,
  };
  const input: React.CSSProperties = {
    width: "100%",
    maxWidth: 360,
    padding: "8px 12px",
    borderRadius: 6,
    border: "1px solid var(--color-border-default)",
    background: "var(--color-bg-elevated)",
    color: "var(--color-text-primary)",
    fontSize: 13,
    outline: "none",
  };
  const row: React.CSSProperties = { marginBottom: 22 };

  return (
    <div style={{ padding: 24, maxWidth: 720 }}>
      <PageHeader
        title="Shared Settings"
        description="Filesystem handling settings that apply to all media-manager services (Prism, Pilot). Changes sync within 30 seconds."
      />

      {isLoading ? (
        <div style={{ ...card, height: 240 }} className="skeleton" />
      ) : (
        <div style={card}>
          <div style={row}>
            <label style={label}>Colon replacement</label>
            <select
              style={input}
              value={colonReplacement}
              onChange={(e) => setColonReplacement(e.target.value as ColonReplacement)}
            >
              {colonOptions.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </select>
            <div style={hint}>
              {colonOptions.find((o) => o.value === colonReplacement)?.hint}{" "}
              How to rewrite colons in filenames — required by most filesystems.
            </div>
          </div>

          <div style={row}>
            <label style={{ ...label, display: "flex", alignItems: "center", gap: 8, cursor: "pointer" }}>
              <input
                type="checkbox"
                checked={renameFiles}
                onChange={(e) => setRenameFiles(e.target.checked)}
              />
              Rename files on import
            </label>
            <div style={hint}>
              When imported, files are renamed to match each service&apos;s naming template.
              Disable to keep original filenames.
            </div>
          </div>

          <div style={row}>
            <label style={{ ...label, display: "flex", alignItems: "center", gap: 8, cursor: "pointer" }}>
              <input
                type="checkbox"
                checked={importExtraFiles}
                onChange={(e) => setImportExtraFiles(e.target.checked)}
              />
              Import extra files
            </label>
            <div style={hint}>
              Copy subtitle and metadata files alongside media during import.
            </div>
          </div>

          {importExtraFiles && (
            <div style={row}>
              <label style={label}>Extra file extensions</label>
              <input
                type="text"
                style={input}
                value={extraFileExtensions}
                onChange={(e) => setExtraFileExtensions(e.target.value)}
                placeholder="srt,nfo"
              />
              <div style={hint}>Comma-separated list, no dots (e.g. <code>srt,nfo,en.srt</code>).</div>
            </div>
          )}

          <div style={{ display: "flex", justifyContent: "flex-end", marginTop: 4 }}>
            <button
              onClick={handleSave}
              disabled={update.isPending}
              style={{
                padding: "8px 18px",
                background: "var(--color-accent)",
                color: "var(--color-accent-fg)",
                border: "none",
                borderRadius: 6,
                fontSize: 13,
                fontWeight: 600,
                cursor: update.isPending ? "wait" : "pointer",
              }}
            >
              {update.isPending ? "Saving..." : "Save"}
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
