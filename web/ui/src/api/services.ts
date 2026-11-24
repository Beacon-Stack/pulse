import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { apiFetch } from "./client";
import type { Service, RegisterServiceInput } from "@/types";

export function useServices() {
  return useQuery({
    queryKey: ["services"],
    queryFn: () => apiFetch<Service[]>("/services"),
  });
}

export function useService(id: string) {
  return useQuery({
    queryKey: ["services", id],
    queryFn: () => apiFetch<Service>(`/services/${id}`),
    enabled: !!id,
  });
}

export function useRegisterService() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: RegisterServiceInput) =>
      apiFetch<Service>("/services/register", {
        method: "POST",
        body: JSON.stringify(input),
      }),
    onSuccess: (svc) => {
      qc.invalidateQueries({ queryKey: ["services"] });
      toast.success(`Registered ${svc.name}`);
    },
    onError: (err) => toast.error((err as Error).message),
  });
}

export function useDeregisterService() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      apiFetch<void>(`/services/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["services"] });
      toast.success("Service deregistered");
    },
    onError: (err) => toast.error((err as Error).message),
  });
}
