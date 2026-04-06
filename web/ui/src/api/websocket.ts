import { useEffect, useRef } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";

interface ServerEvent {
  type: string;
  timestamp: string;
  service_id?: string;
  data?: Record<string, unknown>;
}

function buildWsUrl(): string {
  const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${proto}//${window.location.host}/api/v1/ws`;
}

export function useWebSocket() {
  const qc = useQueryClient();
  const retryDelay = useRef(1000);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const wsRef = useRef<WebSocket | null>(null);

  useEffect(() => {
    let stopped = false;

    function connect() {
      if (stopped) return;

      const ws = new WebSocket(buildWsUrl());
      wsRef.current = ws;

      ws.onopen = () => {
        retryDelay.current = 1000;
      };

      ws.onmessage = (ev) => {
        let event: ServerEvent;
        try {
          event = JSON.parse(ev.data as string) as ServerEvent;
        } catch {
          return;
        }
        handleEvent(event);
      };

      ws.onclose = () => {
        wsRef.current = null;
        if (stopped) return;
        const delay = retryDelay.current;
        retryDelay.current = Math.min(delay * 2, 30_000);
        timerRef.current = setTimeout(connect, delay);
      };

      ws.onerror = () => {
        ws.close();
      };
    }

    function handleEvent(e: ServerEvent) {
      switch (e.type) {
        case "service_registered":
        case "service_deregistered":
          qc.invalidateQueries({ queryKey: ["services"] });
          toast.info(`Service ${e.data?.action === "created" ? "registered" : "updated"}: ${e.data?.name}`);
          break;

        case "service_online":
          qc.invalidateQueries({ queryKey: ["services"] });
          toast.success(`${e.data?.name} is online`);
          break;

        case "service_offline":
          qc.invalidateQueries({ queryKey: ["services"] });
          toast.error(`${e.data?.name} went offline`);
          break;

        case "service_degraded":
          qc.invalidateQueries({ queryKey: ["services"] });
          toast.warning(`${e.data?.name} is degraded`);
          break;

        case "config_updated":
        case "config_deleted":
          qc.invalidateQueries({ queryKey: ["config"] });
          break;

        case "indexer_created":
        case "indexer_updated":
        case "indexer_deleted":
        case "indexer_assigned":
        case "indexer_unassigned":
          qc.invalidateQueries({ queryKey: ["indexers"] });
          break;
      }
    }

    connect();

    return () => {
      stopped = true;
      if (timerRef.current !== null) clearTimeout(timerRef.current);
      wsRef.current?.close();
    };
  }, [qc]);
}
