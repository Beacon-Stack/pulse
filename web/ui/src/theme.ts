// ── Theme system ──────────────────────────────────────────────────────────────
// Copied from Luminarr — same presets, same CSS custom property approach.
// Storage keys use "pulse-" prefix to avoid collisions.

export type ThemeMode = "dark" | "light" | "system";

export interface ThemePreset {
  id: string;
  label: string;
  mode: "dark" | "light";
  preview: { bg: string; surface: string; accent: string; text: string };
  vars: {
    "bg-base": string;
    "bg-surface": string;
    "bg-elevated": string;
    "bg-subtle": string;
    "border-subtle": string;
    "border-default": string;
    "border-strong": string;
    accent: string;
    "accent-hover": string;
    "accent-muted": string;
    "accent-fg": string;
    "text-primary": string;
    "text-secondary": string;
    "text-muted": string;
    success: string;
    warning: string;
    danger: string;
  };
}

export const THEME_PRESETS: ThemePreset[] = [
  {
    id: "pulse", label: "Pulse", mode: "dark",
    preview: { bg: "#0d0d12", surface: "#13131a", accent: "#7c6af7", text: "#f0f0f5" },
    vars: { "bg-base": "#0d0d12", "bg-surface": "#13131a", "bg-elevated": "#1c1c27", "bg-subtle": "#252535", "border-subtle": "rgba(255,255,255,0.078)", "border-default": "rgba(255,255,255,0.133)", "border-strong": "rgba(255,255,255,0.267)", accent: "#7c6af7", "accent-hover": "#9283f9", "accent-muted": "rgba(124,106,247,0.12)", "accent-fg": "#ffffff", "text-primary": "#f0f0f5", "text-secondary": "#9898b0", "text-muted": "#5a5a72", success: "#34d399", warning: "#fbbf24", danger: "#f87171" },
  },
  {
    id: "catppuccin-mocha", label: "Catppuccin Mocha", mode: "dark",
    preview: { bg: "#1e1e2e", surface: "#181825", accent: "#cba6f7", text: "#cdd6f4" },
    vars: { "bg-base": "#1e1e2e", "bg-surface": "#181825", "bg-elevated": "#313244", "bg-subtle": "#45475a", "border-subtle": "rgba(203,166,247,0.10)", "border-default": "rgba(203,166,247,0.16)", "border-strong": "rgba(203,166,247,0.30)", accent: "#cba6f7", "accent-hover": "#b4befe", "accent-muted": "rgba(203,166,247,0.12)", "accent-fg": "#1e1e2e", "text-primary": "#cdd6f4", "text-secondary": "#bac2de", "text-muted": "#7f849c", success: "#a6e3a1", warning: "#f9e2af", danger: "#f38ba8" },
  },
  {
    id: "dracula", label: "Dracula", mode: "dark",
    preview: { bg: "#1e1f29", surface: "#282a36", accent: "#bd93f9", text: "#f8f8f2" },
    vars: { "bg-base": "#1e1f29", "bg-surface": "#282a36", "bg-elevated": "#343746", "bg-subtle": "#44475a", "border-subtle": "rgba(189,147,249,0.10)", "border-default": "rgba(189,147,249,0.16)", "border-strong": "rgba(189,147,249,0.30)", accent: "#bd93f9", "accent-hover": "#caa9fa", "accent-muted": "rgba(189,147,249,0.12)", "accent-fg": "#282a36", "text-primary": "#f8f8f2", "text-secondary": "#cfcfe2", "text-muted": "#6272a4", success: "#50fa7b", warning: "#f1fa8c", danger: "#ff5555" },
  },
  {
    id: "nord", label: "Nord", mode: "dark",
    preview: { bg: "#232831", surface: "#2e3440", accent: "#88c0d0", text: "#eceff4" },
    vars: { "bg-base": "#232831", "bg-surface": "#2e3440", "bg-elevated": "#3b4252", "bg-subtle": "#434c5e", "border-subtle": "rgba(136,192,208,0.10)", "border-default": "rgba(136,192,208,0.16)", "border-strong": "rgba(136,192,208,0.30)", accent: "#88c0d0", "accent-hover": "#8fbcbb", "accent-muted": "rgba(136,192,208,0.12)", "accent-fg": "#2e3440", "text-primary": "#eceff4", "text-secondary": "#d8dee9", "text-muted": "#4c566a", success: "#a3be8c", warning: "#ebcb8b", danger: "#bf616a" },
  },
  {
    id: "tokyo-night", label: "Tokyo Night", mode: "dark",
    preview: { bg: "#1a1b26", surface: "#16161e", accent: "#7aa2f7", text: "#c0caf5" },
    vars: { "bg-base": "#1a1b26", "bg-surface": "#16161e", "bg-elevated": "#1f2335", "bg-subtle": "#292e42", "border-subtle": "rgba(122,162,247,0.10)", "border-default": "rgba(122,162,247,0.16)", "border-strong": "rgba(122,162,247,0.30)", accent: "#7aa2f7", "accent-hover": "#89b4fa", "accent-muted": "rgba(122,162,247,0.12)", "accent-fg": "#1a1b26", "text-primary": "#c0caf5", "text-secondary": "#a9b1d6", "text-muted": "#565f89", success: "#9ece6a", warning: "#e0af68", danger: "#f7768e" },
  },
  {
    id: "catppuccin-latte", label: "Catppuccin Latte", mode: "light",
    preview: { bg: "#eff1f5", surface: "#e6e9ef", accent: "#7287fd", text: "#4c4f69" },
    vars: { "bg-base": "#eff1f5", "bg-surface": "#e6e9ef", "bg-elevated": "#ccd0da", "bg-subtle": "#bcc0cc", "border-subtle": "rgba(76,79,105,0.10)", "border-default": "rgba(76,79,105,0.16)", "border-strong": "rgba(76,79,105,0.30)", accent: "#7287fd", "accent-hover": "#5c70f0", "accent-muted": "rgba(114,135,253,0.12)", "accent-fg": "#ffffff", "text-primary": "#4c4f69", "text-secondary": "#5c5f77", "text-muted": "#8c8fa1", success: "#40a02b", warning: "#df8e1d", danger: "#d20f39" },
  },
];

export const DEFAULT_DARK_PRESET = "pulse";
export const DEFAULT_LIGHT_PRESET = "catppuccin-latte";

const KEY_MODE = "pulse-theme-mode";
const KEY_DARK = "pulse-theme-dark";
const KEY_LIGHT = "pulse-theme-light";

function applyPreset(preset: ThemePreset): void {
  const root = document.documentElement;
  const v = preset.vars;
  root.style.setProperty("--color-bg-base", v["bg-base"]);
  root.style.setProperty("--color-bg-surface", v["bg-surface"]);
  root.style.setProperty("--color-bg-elevated", v["bg-elevated"]);
  root.style.setProperty("--color-bg-subtle", v["bg-subtle"]);
  root.style.setProperty("--color-border-subtle", v["border-subtle"]);
  root.style.setProperty("--color-border-default", v["border-default"]);
  root.style.setProperty("--color-border-strong", v["border-strong"]);
  root.style.setProperty("--color-accent", v["accent"]);
  root.style.setProperty("--color-accent-hover", v["accent-hover"]);
  root.style.setProperty("--color-accent-muted", v["accent-muted"]);
  root.style.setProperty("--color-accent-fg", v["accent-fg"]);
  root.style.setProperty("--color-text-primary", v["text-primary"]);
  root.style.setProperty("--color-text-secondary", v["text-secondary"]);
  root.style.setProperty("--color-text-muted", v["text-muted"]);
  root.style.setProperty("--color-success", v["success"]);
  root.style.setProperty("--color-warning", v["warning"]);
  root.style.setProperty("--color-danger", v["danger"]);
}

export function resolveMode(mode: ThemeMode): "dark" | "light" {
  if (mode === "system") return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
  return mode;
}

export function getStoredMode(): ThemeMode {
  const raw = localStorage.getItem(KEY_MODE);
  if (raw === "dark" || raw === "light" || raw === "system") return raw;
  return "dark";
}

export function getStoredPreset(resolvedMode: "dark" | "light"): string {
  const key = resolvedMode === "dark" ? KEY_DARK : KEY_LIGHT;
  return localStorage.getItem(key) ?? (resolvedMode === "dark" ? DEFAULT_DARK_PRESET : DEFAULT_LIGHT_PRESET);
}

export function findPreset(id: string): ThemePreset {
  return THEME_PRESETS.find((p) => p.id === id) ?? THEME_PRESETS[0];
}

export function applyTheme(): void {
  const mode = getStoredMode();
  const resolved = resolveMode(mode);
  const presetId = getStoredPreset(resolved);
  applyPreset(findPreset(presetId));
}

export function setThemeMode(mode: ThemeMode): void {
  localStorage.setItem(KEY_MODE, mode);
  applyTheme();
}

export function setThemePreset(resolvedMode: "dark" | "light", presetId: string): void {
  const key = resolvedMode === "dark" ? KEY_DARK : KEY_LIGHT;
  localStorage.setItem(key, presetId);
  const currentResolved = resolveMode(getStoredMode());
  if (currentResolved === resolvedMode) applyPreset(findPreset(presetId));
}
