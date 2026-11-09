import { useCallback, useEffect, useRef } from "react";
import { useLatest } from "./hooks";

/** Posts playback progress and reports a stop when the page is hidden or unmounted. */
export function useProgressReporter(
  itemId: string | undefined,
  duration: number,
  timeRef: { readonly current: number },
  sourceRef: { readonly current: { mode: string } | null },
) {
  const lastReport = useRef(0);
  const report = useCallback(
    (event: "progress" | "stop" | "start", paused = false) => {
      if (!itemId) return;
      const body = JSON.stringify({
        position: timeRef.current,
        duration,
        paused,
        event,
        method: sourceRef.current?.mode === "hls" ? "Transcode" : "DirectPlay",
      });
      fetch(`/api/items/${itemId}/progress`, { method: "POST", body, headers: { "Content-Type": "application/json" }, keepalive: true }).catch(() => {});
      lastReport.current = Date.now();
    },
    [itemId, duration, timeRef, sourceRef],
  );
  const reportRef = useLatest(report);
  useEffect(() => {
    const onHide = () => reportRef.current("stop");
    window.addEventListener("pagehide", onHide);
    return () => {
      window.removeEventListener("pagehide", onHide);
      if (timeRef.current > 0) reportRef.current("stop");
    };
  }, [reportRef, timeRef]);
  return { report, lastReport };
}
