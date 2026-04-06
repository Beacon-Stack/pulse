import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { apiFetch } from "./client";

export interface DownloadClient {
  id: string;
  name: string;
  kind: string;
  protocol: string;
  enabled: boolean;
  priority: number;
  host: string;
  port: number;
  use_ssl: boolean;
  username: string;
  category: string;
  directory: string;
  settings: string;
  created_at: string;
  updated_at: string;
}

export interface CreateDownloadClientInput {
  name: string;
  kind: string;
  protocol?: string;
  enabled?: boolean;
  priority?: number;
  host: string;
  port: number;
  use_ssl?: boolean;
  username?: string;
  password?: string;
  category?: string;
  directory?: string;
  settings?: string;
}

export interface TestResult {
  success: boolean;
  message: string;
  duration: string;
}

export function useDownloadClients() {
  return useQuery({
    queryKey: ["download-clients"],
    queryFn: () => apiFetch<DownloadClient[]>("/download-clients"),
  });
}

export function useDownloadClient(id: string) {
  return useQuery({
    queryKey: ["download-clients", id],
    queryFn: () => apiFetch<DownloadClient>(`/download-clients/${id}`),
    enabled: !!id,
  });
}

export function useCreateDownloadClient() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateDownloadClientInput) =>
      apiFetch<DownloadClient>("/download-clients", {
        method: "POST",
        body: JSON.stringify(input),
      }),
    onSuccess: (dc) => {
      qc.invalidateQueries({ queryKey: ["download-clients"] });
      toast.success(`Added ${dc.name}`);
    },
    onError: (err) => toast.error((err as Error).message),
  });
}

export function useUpdateDownloadClient() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...input }: CreateDownloadClientInput & { id: string }) =>
      apiFetch<DownloadClient>(`/download-clients/${id}`, {
        method: "PUT",
        body: JSON.stringify(input),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["download-clients"] });
      toast.success("Download client updated");
    },
    onError: (err) => toast.error((err as Error).message),
  });
}

export function useDeleteDownloadClient() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      apiFetch<void>(`/download-clients/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["download-clients"] });
      toast.success("Download client deleted");
    },
    onError: (err) => toast.error((err as Error).message),
  });
}

export function useTestDownloadClient() {
  return useMutation({
    mutationFn: (input: { kind: string; host: string; port: number; use_ssl?: boolean }) =>
      apiFetch<TestResult>("/download-clients/test", {
        method: "POST",
        body: JSON.stringify(input),
      }),
  });
}
