import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { apiFetch } from "./client";
import type { Indexer, CreateIndexerInput, IndexerAssignment } from "@/types";

export function useIndexers() {
  return useQuery({
    queryKey: ["indexers"],
    queryFn: () => apiFetch<Indexer[]>("/indexers"),
  });
}

export function useIndexer(id: string) {
  return useQuery({
    queryKey: ["indexers", id],
    queryFn: () => apiFetch<Indexer>(`/indexers/${id}`),
    enabled: !!id,
  });
}

export function useCreateIndexer() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateIndexerInput) =>
      apiFetch<Indexer>("/indexers", {
        method: "POST",
        body: JSON.stringify(input),
      }),
    onSuccess: (idx) => {
      qc.invalidateQueries({ queryKey: ["indexers"] });
      toast.success(`Created indexer: ${idx.name}`);
    },
    onError: (err) => toast.error((err as Error).message),
  });
}

export function useUpdateIndexer() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...input }: CreateIndexerInput & { id: string }) =>
      apiFetch<Indexer>(`/indexers/${id}`, {
        method: "PUT",
        body: JSON.stringify(input),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["indexers"] });
      toast.success("Indexer updated");
    },
    onError: (err) => toast.error((err as Error).message),
  });
}

export function useDeleteIndexer() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      apiFetch<void>(`/indexers/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["indexers"] });
      toast.success("Indexer deleted");
    },
    onError: (err) => toast.error((err as Error).message),
  });
}

export function useAssignIndexer() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ indexerId, serviceId, overrides }: { indexerId: string; serviceId: string; overrides?: string }) =>
      apiFetch<IndexerAssignment>(`/indexers/${indexerId}/assign`, {
        method: "POST",
        body: JSON.stringify({ service_id: serviceId, overrides }),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["indexers"] });
      toast.success("Indexer assigned");
    },
    onError: (err) => toast.error((err as Error).message),
  });
}

export function useUnassignIndexer() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ indexerId, serviceId }: { indexerId: string; serviceId: string }) =>
      apiFetch<void>(`/indexers/${indexerId}/assign/${serviceId}`, { method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["indexers"] });
      toast.success("Indexer unassigned");
    },
    onError: (err) => toast.error((err as Error).message),
  });
}

export function useIndexerAssignments(indexerId: string) {
  return useQuery({
    queryKey: ["indexers", indexerId, "assignments"],
    queryFn: () => apiFetch<IndexerAssignment[]>(`/indexers/${indexerId}/assignments`),
    enabled: !!indexerId,
  });
}

export function useIndexersForService(serviceId: string) {
  return useQuery({
    queryKey: ["services", serviceId, "indexers"],
    queryFn: () => apiFetch<Indexer[]>(`/services/${serviceId}/indexers`),
    enabled: !!serviceId,
  });
}
