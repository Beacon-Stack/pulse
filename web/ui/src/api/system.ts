import { useQuery } from "@tanstack/react-query";
import { apiFetch } from "./client";
import type { SystemStatus } from "@/types";

export function useSystemStatus() {
  return useQuery({
    queryKey: ["system", "status"],
    queryFn: () => apiFetch<SystemStatus>("/system/status"),
    refetchInterval: 30_000,
  });
}
