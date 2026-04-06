import { useState } from "react";
import PageHeader from "@/components/PageHeader";
import { card, sectionHeader } from "@/lib/styles";
import {
  THEME_PRESETS,
  getStoredMode,
  getStoredPreset,
  setThemeMode,
  setThemePreset,
  type ThemeMode,
} from "@/theme";

export default function AppSettingsPage() {
  const [mode, setMode] = useState<ThemeMode>(getStoredMode);
  const [darkPreset, setDarkPreset] = useState(() => getStoredPreset("dark"));
  const [lightPreset, setLightPreset] = useState(() => getStoredPreset("light"));

  const darkPresets = THEME_PRESETS.filter((p) => p.mode === "dark");
  const lightPresets = THEME_PRESETS.filter((p) => p.mode === "light");

  return (
    <div style={{ padding: 24, maxWidth: 900 }}>
      <PageHeader title="App Settings" description="Appearance and preferences." />

      <div style={card}>
        <h3 style={sectionHeader}>Theme Mode</h3>
        <div style={{ display: "flex", gap: 8, marginBottom: 24 }}>
          {(["dark", "light", "system"] as ThemeMode[]).map((m) => (
            <button
              key={m}
              onClick={() => { setMode(m); setThemeMode(m); }}
              style={{
                padding: "6px 16px",
                borderRadius: 6,
                border: "1px solid var(--color-border-default)",
                background: mode === m ? "var(--color-accent-muted)" : "transparent",
                color: mode === m ? "var(--color-accent)" : "var(--color-text-secondary)",
                fontSize: 13,
                fontWeight: 500,
                cursor: "pointer",
                textTransform: "capitalize",
              }}
            >
              {m}
            </button>
          ))}
        </div>

        <h3 style={sectionHeader}>Dark Themes</h3>
        <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(160px, 1fr))", gap: 8, marginBottom: 24 }}>
          {darkPresets.map((p) => (
            <button
              key={p.id}
              onClick={() => { setDarkPreset(p.id); setThemePreset("dark", p.id); }}
              style={{
                display: "flex",
                alignItems: "center",
                gap: 8,
                padding: "8px 12px",
                borderRadius: 6,
                border: darkPreset === p.id ? "2px solid var(--color-accent)" : "1px solid var(--color-border-default)",
                background: "var(--color-bg-elevated)",
                color: "var(--color-text-primary)",
                fontSize: 12,
                cursor: "pointer",
                textAlign: "left",
              }}
            >
              <div style={{ display: "flex", gap: 2, flexShrink: 0 }}>
                {[p.preview.bg, p.preview.surface, p.preview.accent, p.preview.text].map((c, i) => (
                  <div key={i} style={{ width: 12, height: 12, borderRadius: 3, background: c }} />
                ))}
              </div>
              {p.label}
            </button>
          ))}
        </div>

        <h3 style={sectionHeader}>Light Themes</h3>
        <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(160px, 1fr))", gap: 8 }}>
          {lightPresets.map((p) => (
            <button
              key={p.id}
              onClick={() => { setLightPreset(p.id); setThemePreset("light", p.id); }}
              style={{
                display: "flex",
                alignItems: "center",
                gap: 8,
                padding: "8px 12px",
                borderRadius: 6,
                border: lightPreset === p.id ? "2px solid var(--color-accent)" : "1px solid var(--color-border-default)",
                background: "var(--color-bg-elevated)",
                color: "var(--color-text-primary)",
                fontSize: 12,
                cursor: "pointer",
                textAlign: "left",
              }}
            >
              <div style={{ display: "flex", gap: 2, flexShrink: 0 }}>
                {[p.preview.bg, p.preview.surface, p.preview.accent, p.preview.text].map((c, i) => (
                  <div key={i} style={{ width: 12, height: 12, borderRadius: 3, background: c }} />
                ))}
              </div>
              {p.label}
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}
