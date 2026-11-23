import type { IndexerStats } from "@/types";

// Plain-language summaries of an indexer's search stats, e.g.
// "1.8s per search · 34 results · ~120 seeders · 12% only here · 97% success".

const errorLabels: Record<string, string> = {
  timeout: "timed out",
  unreachable: "unreachable",
  http_error: "site error",
  cloudflare_no_solver: "Cloudflare, no FlareSolverr",
  cloudflare_solve_failed: "Cloudflare not solved",
  ip_banned: "IP blocked",
  other: "other error",
};

export function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  if (ms < 10_000) return `${(ms / 1000).toFixed(1)}s`;
  return `${Math.round(ms / 1000)}s`;
}

export const pct = (x: number) => `${Math.round(x * 100)}%`;

export interface Part {
  text: string;
  tone?: "good" | "warn" | "bad";
}

export function statsParts(st: IndexerStats): Part[] {
  const ok = st.searches - st.failures;
  const parts: Part[] = [];
  if (ok > 0) {
    parts.push({
      text: `${formatDuration(st.median_ms)} per search`,
      tone: st.median_ms <= 3000 ? "good" : st.median_ms > 15000 ? "warn" : undefined,
    });
    parts.push({ text: `${Math.round(st.avg_results)} results` });
    if (st.median_seeders > 0) parts.push({ text: `~${st.median_seeders} seeders` });
  }
  if (st.comparisons > 0) {
    parts.push({
      text: `${pct(st.unique_share)} only here`,
      tone: st.unique_share >= 0.2 ? "good" : undefined,
    });
  }
  if (st.behind_cloudflare >= 0.5) {
    parts.push({
      text: `Cloudflare (${pct(st.solve_rate)} solved)`,
      tone: st.solve_rate < 0.8 ? "warn" : undefined,
    });
  }
  parts.push({
    text: `${pct(st.success_rate)} success`,
    tone: st.success_rate < 0.5 ? "bad" : st.success_rate < 0.9 ? "warn" : undefined,
  });
  return parts;
}

export function statsTooltip(st: IndexerStats, windowDays: number): string {
  const lines = [
    `Last ${windowDays} days, from real searches:`,
    `${st.searches} searches, ${st.failures} failed`,
  ];
  if (st.searches > st.failures) {
    lines.push(
      `Typical search ${formatDuration(st.median_ms)}, slow ones ${formatDuration(st.p90_ms)}`,
      `${st.avg_results.toFixed(1)} results per search, ${st.avg_with_seeders.toFixed(1)} with seeders`,
      `Best results typically have ${st.median_seeders} seeders`,
    );
  }
  if (st.comparisons > 0) {
    lines.push(
      `Compared with other indexers ${st.comparisons} times: ${pct(st.unique_share)} of its results ` +
        `(${st.avg_unique.toFixed(1)} per search) were found nowhere else`,
    );
  }
  if (st.behind_cloudflare > 0) {
    lines.push(`Behind Cloudflare on ${pct(st.behind_cloudflare)} of searches, ${pct(st.solve_rate)} solved`);
  }
  for (const [kind, n] of Object.entries(st.error_kinds ?? {})) {
    lines.push(`${n} × ${errorLabels[kind] ?? kind}`);
  }
  return lines.join("\n");
}
