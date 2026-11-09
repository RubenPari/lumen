import { useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";
import type { ScanStatus } from "./api";

/** Subscribes to server-sent events and keeps cached data fresh. */
export function useServerEvents() {
  const qc = useQueryClient();
  useEffect(() => {
    let es: EventSource | null = null;
    let timer: number | undefined;
    let pending = false;
    const refreshLibrary = () => {
      // Coalesce bursts of library changes.
      if (pending) return;
      pending = true;
      timer = window.setTimeout(() => {
        pending = false;
        for (const key of ["home", "items", "libraries", "children", "item", "genres", "dashboard", "similar"]) qc.invalidateQueries({ queryKey: [key] });
      }, 600);
    };
    const connect = () => {
      es = new EventSource("/api/events");
      es.onmessage = (m) => {
        let e: { type: string; data?: unknown };
        try {
          e = JSON.parse(m.data);
        } catch {
          return;
        }
        switch (e.type) {
          case "library.changed":
          case "livetv.changed":
            refreshLibrary();
            break;
          case "scan.progress":
            qc.setQueryData(["scan"], e.data as ScanStatus);
            if (!(e.data as ScanStatus).running) refreshLibrary();
            break;
          case "userdata.changed":
            qc.invalidateQueries({ queryKey: ["home"] });
            qc.invalidateQueries({ queryKey: ["item"] });
            qc.invalidateQueries({ queryKey: ["children"] });
            break;
        }
      };
    };
    connect();
    return () => {
      es?.close();
      window.clearTimeout(timer);
    };
  }, [qc]);
}
