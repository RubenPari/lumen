import { useQuery } from "@tanstack/react-query";
import clsx from "clsx";
import { ArrowLeft, BookOpen, ChevronLeft, ChevronRight, Columns2, Minus, Plus, Settings2 } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { Spinner } from "../components/ui";
import { api, type Item } from "../lib/api";
import { useLatest } from "../lib/hooks";
import { t } from "../lib/i18n";

type Theme = "dark" | "sepia" | "light";
const themes: Record<Theme, { bg: string; fg: string }> = {
  dark: { bg: "#111219", fg: "#e8e9ee" },
  sepia: { bg: "#f4ecd8", fg: "#3b2f20" },
  light: { bg: "#ffffff", fg: "#1a1a1a" },
};

interface ReaderPrefs {
  theme: Theme;
  fontSize: number;
  spread: boolean;
  rtl: boolean;
}

function loadPrefs(): ReaderPrefs {
  try {
    return { theme: "dark", fontSize: 110, spread: true, rtl: false, ...JSON.parse(localStorage.getItem("lumen.reader") || "{}") };
  } catch {
    return { theme: "dark", fontSize: 110, spread: true, rtl: false };
  }
}

/** Debounced saving of the reading position. */
function useSaveProgress(id?: string) {
  const timer = useRef<number | undefined>(undefined);
  const last = useRef<{ percent: number; location: string } | null>(null);
  const flush = useCallback(() => {
    if (!id || !last.current) return;
    fetch(`/api/items/${id}/reading`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(last.current),
      keepalive: true,
    }).catch(() => {});
    last.current = null;
  }, [id]);
  useEffect(() => {
    window.addEventListener("pagehide", flush);
    return () => {
      window.removeEventListener("pagehide", flush);
      flush();
    };
  }, [flush]);
  return useCallback(
    (percent: number, location: string) => {
      last.current = { percent: Math.round(percent * 10) / 10, location };
      window.clearTimeout(timer.current);
      timer.current = window.setTimeout(flush, 1500);
    },
    [flush],
  );
}

export function Reader() {
  const { id } = useParams();
  const navigate = useNavigate();
  const { data: item, error } = useQuery({ queryKey: ["item", id], queryFn: () => api.get<Item>(`/api/items/${id}`), staleTime: Infinity });
  const [prefs, setPrefsState] = useState<ReaderPrefs>(loadPrefs);
  const [chrome, setChrome] = useState(true);
  const [settings, setSettings] = useState(false);
  const [progress, setProgress] = useState<{ label: string; percent: number }>({ label: "", percent: 0 });
  const setPrefs = (p: Partial<ReaderPrefs>) => {
    const n = { ...prefs, ...p };
    setPrefsState(n);
    try {
      localStorage.setItem("lumen.reader", JSON.stringify(n));
    } catch {
      /* ignore */
    }
  };
  const save = useSaveProgress(item?.id);
  const onProgress = useCallback(
    (percent: number, location: string, label: string) => {
      setProgress({ percent, label });
      save(percent, location);
    },
    [save],
  );
  useEffect(() => {
    if (!chrome || settings) return;
    const tm = setTimeout(() => setChrome(false), 3500);
    return () => clearTimeout(tm);
  }, [chrome, settings, progress]);

  if (error) return <div className="grid h-dvh place-items-center text-ink-300">{(error as Error).message}</div>;
  if (!item)
    return (
      <div className="grid h-dvh place-items-center bg-ink-950">
        <Spinner />
      </div>
    );
  const kind = (item.container ?? "").toLowerCase();
  const th = themes[prefs.theme];
  return (
    <div className="fixed inset-0 z-[90] flex flex-col" style={{ background: th.bg, color: th.fg }} onMouseMove={(e) => e.clientY < 80 && setChrome(true)}>
      <div
        className={clsx(
          "absolute inset-x-0 top-0 z-20 flex items-center gap-3 px-3 py-3 transition-all duration-300 sm:px-6",
          chrome ? "translate-y-0 opacity-100" : "pointer-events-none -translate-y-full opacity-0",
        )}
        style={{ background: `linear-gradient(${th.bg}, ${th.bg}ee 70%, transparent)` }}
      >
        <button onClick={() => navigate(-1)} className="grid h-10 w-10 place-items-center rounded-full hover:bg-black/10" aria-label={t("Back")}>
          <ArrowLeft size={21} />
        </button>
        <div className="min-w-0 flex-1">
          <div className="truncate font-display font-semibold">{item.name}</div>
          <div className="truncate text-xs opacity-60">{[item.artists?.join(", "), progress.label].filter(Boolean).join(" · ")}</div>
        </div>
        <div className="text-sm tabular-nums opacity-70">{Math.round(progress.percent)}%</div>
        <div className="relative">
          <button onClick={() => setSettings((s) => !s)} className="grid h-10 w-10 place-items-center rounded-full hover:bg-black/10" aria-label={t("Settings")}>
            <Settings2 size={20} />
          </button>
          {settings && (
            <div className="absolute top-12 right-0 w-64 space-y-4 rounded-2xl bg-ink-850 p-4 text-ink-100 shadow-2xl ring-1 ring-white/10">
              <div className="flex gap-2">
                {(Object.keys(themes) as Theme[]).map((k) => (
                  <button
                    key={k}
                    onClick={() => setPrefs({ theme: k })}
                    className={clsx("h-10 flex-1 rounded-xl text-sm font-semibold ring-2", prefs.theme === k ? "ring-accent-400" : "ring-transparent")}
                    style={{ background: themes[k].bg, color: themes[k].fg }}
                  >
                    Aa
                  </button>
                ))}
              </div>
              {kind === "epub" && (
                <div className="flex items-center justify-between">
                  <span className="text-sm text-ink-300">{t("Text size")}</span>
                  <div className="flex items-center gap-2">
                    <button onClick={() => setPrefs({ fontSize: Math.max(70, prefs.fontSize - 10) })} className="grid h-8 w-8 place-items-center rounded-full bg-white/10">
                      <Minus size={15} />
                    </button>
                    <span className="w-10 text-center text-sm tabular-nums">{prefs.fontSize}%</span>
                    <button onClick={() => setPrefs({ fontSize: Math.min(200, prefs.fontSize + 10) })} className="grid h-8 w-8 place-items-center rounded-full bg-white/10">
                      <Plus size={15} />
                    </button>
                  </div>
                </div>
              )}
              {(kind === "cbz" || kind === "cbr" || kind === "pdf") && (
                <button onClick={() => setPrefs({ spread: !prefs.spread })} className="flex w-full items-center justify-between text-sm">
                  <span className="flex items-center gap-2 text-ink-300">
                    <Columns2 size={16} /> {t("Two pages")}
                  </span>
                  <span className={prefs.spread ? "text-accent-400" : "text-ink-500"}>{prefs.spread ? "ON" : "OFF"}</span>
                </button>
              )}
              {(kind === "cbz" || kind === "cbr") && (
                <button onClick={() => setPrefs({ rtl: !prefs.rtl })} className="flex w-full items-center justify-between text-sm">
                  <span className="flex items-center gap-2 text-ink-300">
                    <BookOpen size={16} /> {t("Right to left (manga)")}
                  </span>
                  <span className={prefs.rtl ? "text-accent-400" : "text-ink-500"}>{prefs.rtl ? "ON" : "OFF"}</span>
                </button>
              )}
            </div>
          )}
        </div>
      </div>
      <div className="relative min-h-0 flex-1" onClick={() => setSettings(false)}>
        {kind === "epub" && <EpubReader item={item} prefs={prefs} onProgress={onProgress} onTap={() => setChrome((c) => !c)} />}
        {kind === "pdf" && <PdfReader item={item} prefs={prefs} onProgress={onProgress} onTap={() => setChrome((c) => !c)} />}
        {(kind === "cbz" || kind === "cbr") && <ComicReader item={item} prefs={prefs} onProgress={onProgress} onTap={() => setChrome((c) => !c)} />}
        {!["epub", "pdf", "cbz", "cbr"].includes(kind) && (
          <div className="grid h-full place-items-center p-6 text-center">
            <div>
              <p className="mb-4">{t("This format can't be read in the browser.")}</p>
              <a className="underline" href={`/api/items/${item.id}/file?download=1`}>
                {t("Download")}
              </a>
            </div>
          </div>
        )}
      </div>
      <div className="h-1 w-full bg-black/10">
        <div className="h-full bg-accent-500 transition-[width]" style={{ width: `${progress.percent}%` }} />
      </div>
    </div>
  );
}

type ReaderProps = {
  item: Item;
  prefs: ReaderPrefs;
  onProgress: (percent: number, location: string, label: string) => void;
  onTap: () => void;
};

function useArrowKeys(prev: () => void, next: () => void) {
  const r = useLatest({ prev, next });
  useEffect(() => {
    const k = (e: KeyboardEvent) => {
      if (e.key === "ArrowLeft" || e.key === "PageUp") r.current.prev();
      if (e.key === "ArrowRight" || e.key === "PageDown" || e.key === " ") {
        e.preventDefault();
        r.current.next();
      }
    };
    window.addEventListener("keydown", k);
    return () => window.removeEventListener("keydown", k);
  }, [r]);
}

function NavZones({ prev, next, onTap }: { prev: () => void; next: () => void; onTap: () => void }) {
  return (
    <>
      <button className="group absolute inset-y-0 left-0 z-10 w-[22%] cursor-w-resize" onClick={prev} aria-label="Previous">
        <ChevronLeft className="absolute top-1/2 left-3 opacity-0 transition group-hover:opacity-40" size={34} />
      </button>
      <button className="absolute inset-y-0 left-[22%] z-10 w-[56%]" onClick={onTap} aria-label="Menu" />
      <button className="group absolute inset-y-0 right-0 z-10 w-[22%] cursor-e-resize" onClick={next} aria-label={t("Next")}>
        <ChevronRight className="absolute top-1/2 right-3 opacity-0 transition group-hover:opacity-40" size={34} />
      </button>
    </>
  );
}

// ---- EPUB ----

function EpubReader({ item, prefs, onProgress, onTap }: ReaderProps) {
  const ref = useRef<HTMLDivElement>(null);
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const rendition = useRef<any>(null);
  const [loading, setLoading] = useState(true);
  const prog = useLatest(onProgress);
  const tap = useLatest(onTap);

  useEffect(() => {
    let destroyed = false;
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    let book: any;
    (async () => {
      const [{ default: ePub }, buf] = await Promise.all([import("epubjs"), fetch(`/api/items/${item.id}/file`).then((r) => r.arrayBuffer())]);
      if (destroyed || !ref.current) return;
      book = ePub(buf);
      const r = book.renderTo(ref.current, { width: "100%", height: "100%", flow: "paginated", spread: "auto", allowScriptedContent: false });
      rendition.current = r;
      r.themes.register("lumen", {});
      await r.display(item.userData.progressText || undefined);
      setLoading(false);
      r.on("click", () => tap.current());
      r.on("keyup", (e: KeyboardEvent) => {
        if (e.key === "ArrowLeft") r.prev();
        if (e.key === "ArrowRight") r.next();
      });
      await book.ready;
      await book.locations.generate(1200);
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const report = (loc: any) => {
        const cfi = loc?.start?.cfi;
        if (!cfi) return;
        const pct = (book.locations.percentageFromCfi(cfi) ?? 0) * 100;
        const chapter = book.navigation?.get?.(loc.start.href)?.label?.trim() ?? "";
        prog.current(loc.atEnd ? 100 : pct, cfi, chapter);
      };
      r.on("relocated", report);
      report(r.currentLocation());
    })();
    return () => {
      destroyed = true;
      book?.destroy?.();
    };
  }, [item.id, item.userData.progressText, prog, tap]);

  // Theme & font size
  useEffect(() => {
    const r = rendition.current;
    if (!r) return;
    const th = themes[prefs.theme];
    r.themes.override("color", th.fg);
    r.themes.override("background", th.bg);
    r.themes.fontSize(`${prefs.fontSize}%`);
  }, [prefs.theme, prefs.fontSize, loading]);

  const prev = () => rendition.current?.prev();
  const next = () => rendition.current?.next();
  useArrowKeys(prev, next);
  return (
    <div className="absolute inset-0 px-2 pt-14 pb-6 sm:px-12">
      {loading && (
        <div className="absolute inset-0 grid place-items-center">
          <Spinner />
        </div>
      )}
      <div ref={ref} className="h-full w-full" />
      <button className="absolute inset-y-0 left-0 w-10 opacity-0 sm:w-16" onClick={prev} aria-label="Previous" />
      <button className="absolute inset-y-0 right-0 w-10 opacity-0 sm:w-16" onClick={next} aria-label={t("Next")} />
    </div>
  );
}

// ---- PDF ----

function PdfReader({ item, prefs, onProgress, onTap }: ReaderProps) {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const [doc, setDoc] = useState<any>(null);
  const [page, setPage] = useState(() => Math.max(1, parseInt(item.userData.progressText || "1") || 1));
  const wrap = useRef<HTMLDivElement>(null);
  const canvases = useRef<(HTMLCanvasElement | null)[]>([]);
  const [size, setSize] = useState({ w: 0, h: 0 });
  useEffect(() => {
    let cancelled = false;
    (async () => {
      const pdfjs = await import("pdfjs-dist");
      pdfjs.GlobalWorkerOptions.workerSrc = new URL("pdfjs-dist/build/pdf.worker.min.mjs", import.meta.url).toString();
      const d = await pdfjs.getDocument({ url: `/api/items/${item.id}/file`, withCredentials: true }).promise;
      if (!cancelled) setDoc(d);
    })();
    return () => {
      cancelled = true;
    };
  }, [item.id]);
  useEffect(() => {
    const el = wrap.current;
    if (!el) return;
    const ro = new ResizeObserver(() => setSize({ w: el.clientWidth, h: el.clientHeight }));
    ro.observe(el);
    return () => ro.disconnect();
  }, []);
  const spread = prefs.spread && size.w > size.h * 1.2;
  const pages = doc ? [page, ...(spread && page + 1 <= doc.numPages ? [page + 1] : [])] : [];
  useEffect(() => {
    if (!doc || !size.w) return;
    let cancelled = false;
    (async () => {
      for (let i = 0; i < pages.length; i++) {
        const p = await doc.getPage(pages[i]);
        if (cancelled) return;
        const vp1 = p.getViewport({ scale: 1 });
        const scale = Math.min(size.h / vp1.height, size.w / pages.length / vp1.width) * (window.devicePixelRatio || 1);
        const vp = p.getViewport({ scale });
        const c = canvases.current[i];
        if (!c) continue;
        c.width = vp.width;
        c.height = vp.height;
        c.style.width = `${vp.width / (window.devicePixelRatio || 1)}px`;
        c.style.height = `${vp.height / (window.devicePixelRatio || 1)}px`;
        await p.render({ canvasContext: c.getContext("2d")!, viewport: vp, canvas: c }).promise;
      }
    })();
    onProgress(((page + pages.length - 1) / doc.numPages) * 100, String(page), `${page}/${doc.numPages}`);
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [doc, page, size, spread]);
  const step = spread ? 2 : 1;
  const prev = () => setPage((p) => Math.max(1, p - step));
  const next = () => doc && setPage((p) => Math.min(doc.numPages, p + step));
  useArrowKeys(prev, next);
  return (
    <div className="absolute inset-0 pt-14 pb-4">
      <div ref={wrap} className="flex h-full w-full items-center justify-center gap-1">
        {!doc && <Spinner />}
        {pages.map((p, i) => (
          <canvas key={p} ref={(el) => void (canvases.current[i] = el)} className="shadow-2xl" />
        ))}
      </div>
      <NavZones prev={prev} next={next} onTap={onTap} />
    </div>
  );
}

// ---- Comics ----

function ComicReader({ item, prefs, onProgress, onTap }: ReaderProps) {
  const { data } = useQuery({ queryKey: ["pages", item.id], queryFn: () => api.get<{ count: number }>(`/api/items/${item.id}/pages`) });
  const count = data?.count ?? 0;
  const [page, setPage] = useState(() => Math.max(0, (parseInt(item.userData.progressText || "1") || 1) - 1));
  const [wide, setWide] = useState(window.innerWidth > window.innerHeight * 1.2);
  useEffect(() => {
    const on = () => setWide(window.innerWidth > window.innerHeight * 1.2);
    window.addEventListener("resize", on);
    return () => window.removeEventListener("resize", on);
  }, []);
  // Cover alone, then pairs.
  const spread = prefs.spread && wide && page > 0;
  const shown = count ? [page, ...(spread && page + 1 < count ? [page + 1] : [])] : [];
  useEffect(() => {
    if (!count) return;
    onProgress(((page + shown.length) / count) * 100, String(page + 1), `${page + 1}/${count}`);
    for (const d of [1, 2, 3]) if (page + d < count) new Image().src = `/api/items/${item.id}/pages/${page + d}`;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [page, count, spread]);
  const step = spread ? 2 : 1;
  const fwd = () => setPage((p) => Math.min(Math.max(0, count - 1), p === 0 ? 1 : p + step));
  const back = () => setPage((p) => Math.max(0, p - (prefs.spread && wide && p > 1 ? 2 : 1)));
  const [prev, next] = prefs.rtl ? [fwd, back] : [back, fwd];
  useArrowKeys(prev, next);
  const ordered = prefs.rtl ? [...shown].reverse() : shown;
  return (
    <div className="absolute inset-0">
      <div className="flex h-full w-full items-center justify-center">
        {!count && <Spinner />}
        {ordered.map((p) => (
          <img
            key={p}
            src={`/api/items/${item.id}/pages/${p}`}
            alt={`${p + 1}`}
            className={clsx("max-h-full object-contain select-none", ordered.length > 1 ? "max-w-[50%]" : "max-w-full")}
            draggable={false}
          />
        ))}
      </div>
      <NavZones prev={prev} next={next} onTap={onTap} />
    </div>
  );
}
