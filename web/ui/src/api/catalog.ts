import { useQuery } from "@tanstack/react-query";
import { apiFetch } from "./client";
import type { CatalogResponse } from "@/types";

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
