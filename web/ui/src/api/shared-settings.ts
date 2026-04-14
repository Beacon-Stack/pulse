import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { apiFetch } from "./client";

export type ColonReplacement = "delete" | "dash" | "space-dash" | "smart";

export interface SharedSettings {
  colon_replacement: ColonReplacement;
  import_extra_files: boolean;
  extra_file_extensions: string;
  rename_files: boolean;
  updated_at: string;
}

export interface UpdateSharedSettingsInput {
  colon_replacement: ColonReplacement;
  import_extra_files: boolean;
  extra_file_extensions: string;
  rename_files: boolean;
}

export function useSharedSettings() {
  return useQuery({
    queryKey: ["shared-settings"],
    queryFn: () => apiFetch<SharedSettings>("/shared-settings"),
  });
}

export function useUpdateSharedSettings() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: UpdateSharedSettingsInput) =>
      apiFetch<SharedSettings>("/shared-settings", {
        method: "PUT",
        body: JSON.stringify(input),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["shared-settings"] });
      toast.success("Saved. Syncing to services.");
    },
    onError: (err) => toast.error((err as Error).message),
  });
}
