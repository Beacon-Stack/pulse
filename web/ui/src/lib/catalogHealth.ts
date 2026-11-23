import type { CatalogSiteHealth, CatalogVerdict } from "@/types";
import { formatDuration, pct, type Part } from "@/lib/indexerStats";

// Plain-language labels for the catalog tester's verdicts and stats.

export const verdictInfo: Record<CatalogVerdict, { label: string; tone: "good" | "warn" | "bad" | "muted"; help: string }> = {
  works: { label: "Works", tone: "good", help: "Found results in its last test search." },
  no_results: { label: "No results", tone: "warn", help: "Answers, but its test searches never found anything." },
  flaky: { label: "Flaky", tone: "warn", help: "Its last test search failed; earlier ones worked." },
  needs_flaresolverr: { label: "Needs FlareSolverr", tone: "warn", help: "Behind Cloudflare, and FlareSolverr isn't set up." },
  blocked: { label: "Blocked", tone: "bad", help: "The site has blocked the IP Pulse searches from." },
  down: { label: "Down", tone: "bad", help: "Failed its last few test searches." },
};

const errorLabels: Record<string, string> = {
  timeout: "timed out",
  unreachable: "unreachable",
  http_error: "site error",
  rate_limited: "asked us to slow down",
  gone: "site gone (410)",
  cloudflare_no_solver: "Cloudflare, no FlareSolverr",
  cloudflare_solve_failed: "Cloudflare not solved",
  ip_banned: "IP blocked",
  other: "error",
};

// True when every address is an I2P or Tor hidden service, which Pulse
// can't reach (the tester skips these).
export function needsOverlayNetwork(urls: string[]): boolean {
  if (urls.length === 0) return false;
  return urls.every((u) => {
    const host = u.toLowerCase().replace(/^[a-z]+:\/\//, "").split("/")[0].split(":")[0];
    return host.endsWith(".i2p") || host.endsWith(".onion");
  });
}

export function errorLabel(kind?: string): string {
  return kind ? errorLabels[kind] ?? kind : "";
}

// "1.8s per search · 34 results · ~120 seeders · 12% only here"
export function siteHealthParts(h: CatalogSiteHealth): Part[] {
  const parts: Part[] = [];
  if (h.tests > h.failures) {
    parts.push({
      text: `${formatDuration(h.median_ms)} per search`,
      tone: h.median_ms <= 3000 ? "good" : h.median_ms > 15000 ? "warn" : undefined,
    });
    parts.push({ text: `${Math.round(h.avg_results)} results` });
    if (h.median_seeders > 0) {
      parts.push({ text: `~${h.median_seeders} seeders`, tone: h.median_seeders >= 50 ? "good" : undefined });
    }
    if (h.comparisons > 0) {
      parts.push({ text: `${pct(h.unique_share)} only here`, tone: h.unique_share >= 0.2 ? "good" : undefined });
    }
  }
  if (h.behind_cloudflare >= 0.5) parts.push({ text: "Cloudflare" });
  if (h.consecutive_failures > 0 && h.last_error_kind) {
    parts.push({ text: errorLabel(h.last_error_kind), tone: "bad" });
  }
  return parts;
}

export function timeAgo(iso?: string): string {
  if (!iso) return "";
  const mins = Math.round((Date.now() - new Date(iso).getTime()) / 60000);
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins} min ago`;
  const hours = Math.round(mins / 60);
  if (hours < 48) return `${hours} h ago`;
  return `${Math.round(hours / 24)} days ago`;
}

export function timeUntil(iso?: string): string {
  if (!iso) return "";
  const mins = Math.round((new Date(iso).getTime() - Date.now()) / 60000);
  if (mins <= 1) return "in a moment";
  if (mins < 60) return `in ${mins} min`;
  return `in ${Math.round(mins / 60)} h`;
}

export function healthTooltip(h: CatalogSiteHealth): string {
  const lines = [
    `${verdictInfo[h.verdict].label}: ${verdictInfo[h.verdict].help}`,
    `Tested ${h.tests} times in the last 30 days, ${h.failures} failed. Last test ${timeAgo(h.last_test_at)}.`,
  ];
  if (h.tests > h.failures) {
    lines.push(
      `Typical search ${formatDuration(h.median_ms)}, ${h.avg_results.toFixed(1)} results, best results ~${h.median_seeders} seeders.`,
    );
  }
  if (h.comparisons > 0) {
    lines.push(`${pct(h.unique_share)} of its results weren't found by any other tested indexer.`);
  }
  if (h.behind_cloudflare > 0) {
    lines.push(`Behind Cloudflare on ${pct(h.behind_cloudflare)} of tests, ${pct(h.solve_rate)} solved.`);
  }
  if (h.last_error) lines.push(`Last error: ${h.last_error}`);
  return lines.join("\n");
}
