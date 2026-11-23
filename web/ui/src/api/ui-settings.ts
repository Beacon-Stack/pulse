import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { apiFetch } from "./client";

// The filters the add-indexer page starts with (App Settings). Empty lists
// mean "any". Stored in Pulse, so they apply in every browser.
export interface AddIndexerDefaults {
  protocols: string[];
  privacies: string[];
  categories: string[];
  languages: string[];
}

export function useAddIndexerDefaults() {
  return useQuery({
    queryKey: ["settings", "add-indexer-defaults"],
    queryFn: () => apiFetch<AddIndexerDefaults>("/settings/add-indexer-defaults"),
    staleTime: 60_000,
  });
}

export function useSaveAddIndexerDefaults() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (d: AddIndexerDefaults) =>
      apiFetch<AddIndexerDefaults>("/settings/add-indexer-defaults", {
        method: "PUT",
        body: JSON.stringify(d),
      }),
    onSuccess: (saved) => {
      qc.setQueryData(["settings", "add-indexer-defaults"], saved);
      toast.success("Add Indexer defaults saved");
    },
    onError: (err) => toast.error((err as Error).message),
  });
}
