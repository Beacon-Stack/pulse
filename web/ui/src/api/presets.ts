import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { apiFetch } from "./client";

interface Preset {
  id: string;
  name: string;
  filters: string; // JSON
  created_at: string;
  updated_at: string;
}

export function usePresets() {
  return useQuery({
    queryKey: ["presets"],
    queryFn: () => apiFetch<Preset[]>("/presets"),
  });
}

export function useSavePreset() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { name: string; filters: string }) =>
      apiFetch<Preset>("/presets", {
        method: "PUT",
        body: JSON.stringify(input),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["presets"] });
      toast.success("Preset saved");
    },
    onError: (err) => toast.error((err as Error).message),
  });
}

export function useDeletePreset() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      apiFetch<void>(`/presets/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["presets"] });
      toast.success("Preset deleted");
    },
    onError: (err) => toast.error((err as Error).message),
  });
}
