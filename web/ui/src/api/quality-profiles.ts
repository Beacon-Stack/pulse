import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { apiFetch } from "./client";

// Quality represents a single quality tier. JSON shape matches Prism/Pilot's
// plugin.Quality Go type so the blobs are interchangeable.
export interface Quality {
  resolution: string;
  source: string;
  codec: string;
  hdr: string;
  audio_codec?: string;
  audio_channels?: string;
  name: string;
}

export interface QualityProfile {
  id: string;
  name: string;
  cutoff_json: string;
  qualities_json: string;
  upgrade_allowed: boolean;
  upgrade_until_json?: string | null;
  min_custom_format_score: number;
  upgrade_until_cf_score: number;
  created_at: string;
  updated_at: string;
}

export interface CreateQualityProfileInput {
  name: string;
  cutoff_json: string;
  qualities_json: string;
  upgrade_allowed: boolean;
  upgrade_until_json?: string | null;
  min_custom_format_score?: number;
  upgrade_until_cf_score?: number;
}

export function useQualityProfiles() {
  return useQuery({
    queryKey: ["quality-profiles"],
    queryFn: () => apiFetch<QualityProfile[]>("/quality-profiles"),
  });
}

export function useCreateQualityProfile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateQualityProfileInput) =>
      apiFetch<QualityProfile>("/quality-profiles", {
        method: "POST",
        body: JSON.stringify(input),
      }),
    onSuccess: (p) => {
      qc.invalidateQueries({ queryKey: ["quality-profiles"] });
      toast.success(`Created ${p.name}`);
    },
    onError: (err) => toast.error((err as Error).message),
  });
}

export function useUpdateQualityProfile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...input }: CreateQualityProfileInput & { id: string }) =>
      apiFetch<QualityProfile>(`/quality-profiles/${id}`, {
        method: "PUT",
        body: JSON.stringify(input),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["quality-profiles"] });
      toast.success("Quality profile updated");
    },
    onError: (err) => toast.error((err as Error).message),
  });
}

export function useDeleteQualityProfile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      apiFetch<void>(`/quality-profiles/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["quality-profiles"] });
      toast.success("Quality profile deleted");
    },
    onError: (err) => toast.error((err as Error).message),
  });
}
