import { useCallback, useEffect, useRef, useState } from "react";
import { useLatest } from "./hooks";

/** Shows controls on poke and hides them after `ms`, unless canHide returns false. */
export function useHideControls(ms: number, opts?: { canHide?: () => boolean; onHide?: () => void }) {
  const [visible, setVisible] = useState(true);
  const timer = useRef<number | undefined>(undefined);
  const optsRef = useLatest(opts);
  const poke = useCallback(() => {
    setVisible(true);
    window.clearTimeout(timer.current);
    timer.current = window.setTimeout(() => {
      const o = optsRef.current;
      if (o?.canHide && !o.canHide()) return;
      setVisible(false);
      o?.onHide?.();
    }, ms);
  }, [ms, optsRef]);
  useEffect(() => () => window.clearTimeout(timer.current), []);
  const show = useCallback(() => setVisible(true), []);
  return { visible, poke, show };
}

export function useFullscreenFlag() {
  const [fullscreen, setFullscreen] = useState(false);
  useEffect(() => {
    const on = () => setFullscreen(!!document.fullscreenElement);
    document.addEventListener("fullscreenchange", on);
    return () => document.removeEventListener("fullscreenchange", on);
  }, []);
  return fullscreen;
}
