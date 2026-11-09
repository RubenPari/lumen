import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useCallback, useEffect, useRef, useState, useSyncExternalStore } from "react";
import { api, ApiError, type Item, type Library, type SystemInfo, type User } from "./api";

export function useSystemInfo() {
  return useQuery({ queryKey: ["system"], queryFn: () => api.get<SystemInfo>("/api/system/info"), staleTime: 60_000 });
}

export function useMe() {
  return useQuery({
    queryKey: ["me"],
    queryFn: async () => {
      try {
        return await api.get<User>("/api/me");
      } catch (e) {
        if (e instanceof ApiError && e.status === 401) return null;
        throw e;
      }
    },
    staleTime: 5 * 60_000,
  });
}

export function useLibraries() {
  return useQuery({ queryKey: ["libraries"], queryFn: () => api.get<Library[]>("/api/libraries"), staleTime: 30_000 });
}

export function useItem(id?: string) {
  return useQuery({ queryKey: ["item", id], queryFn: () => api.get<Item>(`/api/items/${id}`), enabled: !!id });
}

/** Mutations for played/favorite toggles; refreshes every item-related query. */
export function useUserDataActions() {
  const qc = useQueryClient();
  const refresh = () => {
    qc.invalidateQueries({ queryKey: ["item"] });
    qc.invalidateQueries({ queryKey: ["items"] });
    qc.invalidateQueries({ queryKey: ["children"] });
    qc.invalidateQueries({ queryKey: ["home"] });
  };
  const played = useMutation({
    mutationFn: ({ id, value }: { id: string; value: boolean }) =>
      value ? api.post<Item>(`/api/items/${id}/played`) : api.del<Item>(`/api/items/${id}/played`),
    onSuccess: refresh,
  });
  const favorite = useMutation({
    mutationFn: ({ id, value }: { id: string; value: boolean }) =>
      value ? api.post<Item>(`/api/items/${id}/favorite`) : api.del<Item>(`/api/items/${id}/favorite`),
    onSuccess: refresh,
  });
  return {
    setPlayed: (id: string, value: boolean) => played.mutate({ id, value }),
    setFavorite: (id: string, value: boolean) => favorite.mutate({ id, value }),
  };
}

export function useDebounced<T>(value: T, ms = 250): T {
  const [v, setV] = useState(value);
  useEffect(() => {
    const id = setTimeout(() => setV(value), ms);
    return () => clearTimeout(id);
  }, [value, ms]);
  return v;
}

export function useScrolled(threshold = 24) {
  const [scrolled, setScrolled] = useState(false);
  useEffect(() => {
    const on = () => setScrolled(window.scrollY > threshold);
    on();
    window.addEventListener("scroll", on, { passive: true });
    return () => window.removeEventListener("scroll", on);
  }, [threshold]);
  return scrolled;
}

export function useInView<T extends Element>(rootMargin = "600px") {
  const ref = useRef<T | null>(null);
  const [inView, setInView] = useState(false);
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const obs = new IntersectionObserver(([e]) => setInView(e.isIntersecting), { rootMargin });
    obs.observe(el);
    return () => obs.disconnect();
  }, [rootMargin]);
  return [ref, inView] as const;
}

// ---- Preferences (per browser) ----

export interface Prefs {
  subtitleSize: "small" | "medium" | "large";
  subtitleLang: string;
  audioLang: string;
  autoplayNext: boolean;
  volume: number;
  muted: boolean;
}

const defaultPrefs: Prefs = { subtitleSize: "medium", subtitleLang: "", audioLang: "", autoplayNext: true, volume: 1, muted: false };
const listeners = new Set<() => void>();
let prefsCache: Prefs = (() => {
  try {
    return { ...defaultPrefs, ...JSON.parse(localStorage.getItem("lumen.prefs") || "{}") };
  } catch {
    return defaultPrefs;
  }
})();

export function setPrefs(p: Partial<Prefs>) {
  prefsCache = { ...prefsCache, ...p };
  try {
    localStorage.setItem("lumen.prefs", JSON.stringify(prefsCache));
  } catch {
    /* ignore */
  }
  listeners.forEach((l) => l());
}

export function getPrefs() {
  return prefsCache;
}

export function usePrefs(): [Prefs, (p: Partial<Prefs>) => void] {
  const p = useSyncExternalStore(
    (cb) => {
      listeners.add(cb);
      return () => listeners.delete(cb);
    },
    () => prefsCache,
  );
  return [p, setPrefs];
}

export function useLatest<T>(v: T) {
  const r = useRef(v);
  r.current = v;
  return r;
}

export function useHotkey(key: string, fn: (e: KeyboardEvent) => void, opts: { meta?: boolean } = {}) {
  const f = useLatest(fn);
  const handler = useCallback(
    (e: KeyboardEvent) => {
      if (opts.meta && !(e.metaKey || e.ctrlKey)) return;
      if (e.key.toLowerCase() === key.toLowerCase()) f.current(e);
    },
    [key, opts.meta, f],
  );
  useEffect(() => {
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [handler]);
}
