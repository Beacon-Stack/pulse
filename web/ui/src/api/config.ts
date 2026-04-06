import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { apiFetch } from "./client";
import type { ConfigEntry, SetConfigInput } from "@/types";

export function useConfigEntries() {
  return useQuery({
    queryKey: ["config"],
    queryFn: () => apiFetch<ConfigEntry[]>("/config"),
  });
}

export function useConfigNamespaces() {
  return useQuery({
    queryKey: ["config", "namespaces"],
    queryFn: () => apiFetch<string[]>("/config/namespaces"),
  });
}

export function useConfigByNamespace(namespace: string) {
  return useQuery({
    queryKey: ["config", namespace],
    queryFn: () => apiFetch<ConfigEntry[]>(`/config/${namespace}`),
    enabled: !!namespace,
  });
}

export function useSetConfig() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: SetConfigInput) =>
      apiFetch<ConfigEntry>("/config", {
        method: "PUT",
        body: JSON.stringify(input),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["config"] });
      toast.success("Config saved");
    },
    onError: (err) => toast.error((err as Error).message),
  });
}

export function useDeleteConfig() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ namespace, key }: { namespace: string; key: string }) =>
      apiFetch<void>(`/config/${namespace}/${key}`, { method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["config"] });
      toast.success("Config entry deleted");
    },
    onError: (err) => toast.error((err as Error).message),
  });
}
