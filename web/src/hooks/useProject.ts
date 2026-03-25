import { useEffect, useRef, useState, useCallback } from "react";
import type { Project } from "../types";

export function useProject() {
  const [project, setProject] = useState<Project | null>(null);
  const [connected, setConnected] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const fetchProject = useCallback(async () => {
    try {
      const res = await fetch("/api/project");
      if (res.ok) {
        const data: Project = await res.json();
        setProject(data);
      }
    } catch {
      // server not available yet
    }
  }, []);

  const debouncedFetch = useCallback(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(fetchProject, 300);
  }, [fetchProject]);

  useEffect(() => {
    fetchProject();

    const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
    const wsUrl = `${proto}//${window.location.host}/ws`;

    function connect() {
      const ws = new WebSocket(wsUrl);
      wsRef.current = ws;

      ws.onopen = () => setConnected(true);
      ws.onclose = () => {
        setConnected(false);
        setTimeout(connect, 2000);
      };
      ws.onmessage = (event) => {
        try {
          const msg = JSON.parse(event.data);
          if (msg.type === "reload") {
            debouncedFetch();
          }
        } catch {
          // ignore malformed messages
        }
      };
    }

    connect();
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current);
      wsRef.current?.close();
    };
  }, [fetchProject, debouncedFetch]);

  return { project, connected };
}
