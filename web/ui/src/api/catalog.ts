import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { apiFetch } from "./client";
import type { CatalogResponse, CatalogHealthResponse } from "@/types";

export function useCatalog(params?: {
  q?: string;
  protocol?: string;
  privacy?: string;
  category?: string;
  language?: string;
}) {
  const qs = new URLSearchParams();
  if (params?.q) qs.set("q", params.q);
  if (params?.protocol) qs.set("protocol", params.protocol);
  if (params?.privacy) qs.set("privacy", params.privacy);
  if (params?.category) qs.set("category", params.category);
  if (params?.language) qs.set("language", params.language);
  const query = qs.toString();

  return useQuery({
    queryKey: ["catalog", params],
    queryFn: () => apiFetch<CatalogResponse>(`/indexers/catalog${query ? `?${query}` : ""}`),
    staleTime: 5 * 60_000,
  });
}

export function useCatalogLanguages() {
  return useQuery({
    queryKey: ["catalog", "languages"],
    queryFn: () => apiFetch<string[]>("/indexers/catalog/languages"),
    staleTime: 5 * 60_000,
  });
}

// Background test results for catalog indexers. Polls faster while a test
// cycle is running so progress and new results show up.
export function useCatalogHealth() {
  return useQuery({
    queryKey: ["catalog", "health"],
    queryFn: () => apiFetch<CatalogHealthResponse>("/indexers/catalog/health"),
    refetchInterval: (q) => (q.state.data?.status.running ? 15_000 : 60_000),
  });
}

// "Test now" for one catalog indexer. Cloudflare sites can take a minute or two.
export function useTestCatalogIndexer() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (catalogId: string) =>
      apiFetch<void>(`/indexers/catalog/${encodeURIComponent(catalogId)}/test`, { method: "POST" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["catalog", "health"] }),
    onError: (err) => toast.error((err as Error).message),
  });
}
