import { useQuery } from "@tanstack/react-query";
import clsx from "clsx";
import Hls from "hls.js";
import { ArrowLeft, ChevronDown, ChevronUp, List, Maximize, Minimize, Pause, Play, Radio, Volume2, VolumeX } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { Button, Spinner } from "../components/ui";
import { api } from "../lib/api";
import { getPrefs, setPrefs, useLatest } from "../lib/hooks";
import { t } from "../lib/i18n";
import { ChannelLogo, hhmm, useNow, type LiveChannel } from "./LiveTV";

export function LivePlayer() {
  const { id } = useParams();
  const navigate = useNavigate();
  const now = useNow(15_000);
  const { data: channels } = useQuery({ queryKey: ["livetv", "channels"], queryFn: () => api.get<LiveChannel[]>("/api/livetv/channels"), refetchInterval: 60_000 });
  const ch = channels?.find((c) => c.id === id);
  const idx = channels?.findIndex((c) => c.id === id) ?? -1;
  const videoRef = useRef<HTMLVideoElement>(null);
  const wrap = useRef<HTMLDivElement>(null);
  const hlsRef = useRef<Hls | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [playing, setPlaying] = useState(false);
  const [muted, setMuted] = useState(getPrefs().muted);
  const [controls, setControls] = useState(true);
  const [list, setList] = useState(false);
  const [fullscreen, setFullscreen] = useState(false);
  const hide = useRef<number | undefined>(undefined);

  const poke = useCallback(() => {
    setControls(true);
    window.clearTimeout(hide.current);
    hide.current = window.setTimeout(() => setControls(false), 3500);
  }, []);

  useEffect(() => {
    if (!id) return;
    let cancelled = false;
    setLoading(true);
    setError(null);
    hlsRef.current?.destroy();
    hlsRef.current = null;
    (async () => {
      try {
        const r = await api.post<{ url: string }>(`/api/livetv/channels/${id}/hls`);
        if (cancelled) return;
        const v = videoRef.current!;
        if (Hls.isSupported()) {
          const hls = new Hls({ liveSyncDurationCount: 3, enableWorker: true, lowLatencyMode: false });
          hls.loadSource(r.url);
          hls.attachMedia(v);
          hls.on(Hls.Events.ERROR, (_, d) => {
            if (!d.fatal) return;
            if (d.type === Hls.ErrorTypes.NETWORK_ERROR) hls.startLoad();
            else if (d.type === Hls.ErrorTypes.MEDIA_ERROR) hls.recoverMediaError();
            else setError(d.details);
          });
          hlsRef.current = hls;
        } else v.src = r.url;
        v.play().catch(() => setPlaying(false));
      } catch (e) {
        if (!cancelled) {
          setError((e as Error).message);
          setLoading(false);
        }
      }
    })();
    poke();
    return () => {
      cancelled = true;
    };
  }, [id, poke]);

  useEffect(() => () => hlsRef.current?.destroy(), []);
  useEffect(() => {
    if (videoRef.current) videoRef.current.muted = muted;
    setPrefs({ muted });
  }, [muted]);
  useEffect(() => {
    const on = () => setFullscreen(!!document.fullscreenElement);
    document.addEventListener("fullscreenchange", on);
    return () => document.removeEventListener("fullscreenchange", on);
  }, []);

  const zap = useCallback(
    (d: number) => {
      if (!channels?.length) return;
      const n = channels[(idx + d + channels.length) % channels.length];
      navigate(`/live/${n.id}`, { replace: true });
    },
    [channels, idx, navigate],
  );
  const keys = useLatest({ zap, poke });
  useEffect(() => {
    const k = (e: KeyboardEvent) => {
      if (e.key === "ArrowUp" || e.key === "PageUp") keys.current.zap(-1);
      else if (e.key === "ArrowDown" || e.key === "PageDown") keys.current.zap(1);
      else if (e.key === "m") setMuted((x) => !x);
      else if (e.key === "f") document.fullscreenElement ? document.exitFullscreen() : wrap.current?.requestFullscreen();
      else if (e.key === " ") {
        e.preventDefault();
        const v = videoRef.current!;
        v.paused ? v.play() : v.pause();
      } else if (e.key === "Escape" && !document.fullscreenElement) navigate("/livetv");
      else return;
      keys.current.poke();
    };
    window.addEventListener("keydown", k);
    return () => window.removeEventListener("keydown", k);
  }, [keys, navigate]);

  const p = ch?.now;
  const pct = p ? Math.min(100, ((now - p.start) / (p.stop - p.start)) * 100) : 0;
  return (
    <div ref={wrap} className={clsx("fixed inset-0 z-[90] bg-black select-none", !controls && "cursor-none")} onMouseMove={poke} onTouchStart={poke}>
      <video
        ref={videoRef}
        className="absolute inset-0 h-full w-full"
        playsInline
        onClick={() => setList(false)}
        onPlaying={() => {
          setLoading(false);
          setPlaying(true);
        }}
        onPause={() => setPlaying(false)}
        onWaiting={() => setLoading(true)}
      />
      {loading && !error && (
        <div className="pointer-events-none absolute inset-0 grid place-items-center">
          <Spinner className="h-14 w-14 border-[3px]" />
        </div>
      )}
      {error && (
        <div className="absolute inset-0 grid place-items-center bg-black/70 p-6 text-center">
          <div>
            <h2 className="mb-2 font-display text-2xl font-semibold">{t("Playback error")}</h2>
            <p className="mb-5 max-w-md text-ink-300">{error}</p>
            <Button onClick={() => navigate("/livetv")}>{t("Back")}</Button>
          </div>
        </div>
      )}

      <div className={clsx("absolute inset-x-0 top-0 flex items-center gap-4 bg-gradient-to-b from-black/80 to-transparent p-4 pb-16 transition-opacity duration-300 sm:p-6", controls ? "opacity-100" : "pointer-events-none opacity-0")}>
        <button onClick={() => navigate("/livetv")} className="grid h-11 w-11 shrink-0 place-items-center rounded-full bg-white/10 backdrop-blur hover:bg-white/20" aria-label={t("Back")}>
          <ArrowLeft size={22} />
        </button>
        {ch && <ChannelLogo ch={ch} className="h-11 w-14" />}
        <div className="min-w-0 flex-1">
          <div className="truncate font-display text-xl font-semibold">
            <span className="mr-2 text-ink-400 tabular-nums">{ch?.number}</span>
            {ch?.name}
          </div>
          <div className="flex items-center gap-2 text-xs font-bold tracking-wider text-red-400 uppercase">
            <span className="h-2 w-2 animate-pulse rounded-full bg-red-500" /> Live
          </div>
        </div>
        <button onClick={() => setList((l) => !l)} className={clsx("grid h-11 w-11 place-items-center rounded-full backdrop-blur hover:bg-white/20", list ? "bg-white text-ink-950" : "bg-white/10")} aria-label={t("Channels")}>
          <List size={20} />
        </button>
      </div>

      {list && channels && (
        <div className="absolute top-24 right-4 bottom-36 w-80 animate-fade-in overflow-y-auto rounded-2xl bg-ink-900/90 p-2 ring-1 ring-white/10 backdrop-blur-xl sm:right-6">
          {channels.map((c) => (
            <button
              key={c.id}
              onClick={() => navigate(`/live/${c.id}`, { replace: true })}
              className={clsx("flex w-full items-center gap-3 rounded-xl p-2 text-left transition", c.id === id ? "bg-white/10" : "hover:bg-white/5")}
            >
              <ChannelLogo ch={c} className="h-9 w-11" />
              <span className="min-w-0">
                <span className="block truncate text-sm font-medium">
                  <span className="mr-1.5 text-ink-500 tabular-nums">{c.number}</span>
                  {c.name}
                </span>
                <span className="block truncate text-xs text-ink-400">{c.now?.title ?? "—"}</span>
              </span>
            </button>
          ))}
        </div>
      )}

      <div className={clsx("absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/90 via-black/50 to-transparent px-4 pt-24 pb-5 transition-opacity duration-300 sm:px-8 sm:pb-7", controls ? "opacity-100" : "pointer-events-none opacity-0")}>
        {p && (
          <div className="mb-4 max-w-3xl">
            <div className="mb-1 text-xs text-ink-300 tabular-nums">
              {hhmm(p.start)} – {hhmm(p.stop)}
              {p.category ? ` · ${p.category}` : ""}
            </div>
            <div className="font-display text-2xl font-semibold">{p.title}</div>
            {p.description && <p className="mt-1 line-clamp-2 text-sm text-ink-300">{p.description}</p>}
            <div className="mt-3 h-1 w-full overflow-hidden rounded-full bg-white/15">
              <div className="h-full bg-red-500" style={{ width: `${pct}%` }} />
            </div>
            {ch?.next && (
              <div className="mt-2 text-xs text-ink-400">
                {t("Later")}: {hhmm(ch.next.start)} · {ch.next.title}
              </div>
            )}
          </div>
        )}
        <div className="flex items-center gap-1">
          <Ctl label={playing ? "Pause" : t("Play")} onClick={() => (videoRef.current?.paused ? videoRef.current.play() : videoRef.current?.pause())}>
            {playing ? <Pause size={24} fill="white" /> : <Play size={24} fill="white" />}
          </Ctl>
          <Ctl label={t("Previous channel")} onClick={() => zap(-1)}>
            <ChevronUp size={24} />
          </Ctl>
          <Ctl label={t("Next channel")} onClick={() => zap(1)}>
            <ChevronDown size={24} />
          </Ctl>
          <Ctl label="Mute" onClick={() => setMuted((m) => !m)}>
            {muted ? <VolumeX size={22} /> : <Volume2 size={22} />}
          </Ctl>
          <span className="ml-2 inline-flex items-center gap-1.5 rounded-full bg-red-500/90 px-2.5 py-1 text-[11px] font-bold tracking-wider uppercase">
            <Radio size={12} /> Live
          </span>
          <div className="flex-1" />
          <Ctl label="Fullscreen" onClick={() => (document.fullscreenElement ? document.exitFullscreen() : wrap.current?.requestFullscreen())}>
            {fullscreen ? <Minimize size={22} /> : <Maximize size={22} />}
          </Ctl>
        </div>
      </div>
    </div>
  );
}

function Ctl({ label, onClick, children }: { label: string; onClick: () => void; children: React.ReactNode }) {
  return (
    <button aria-label={label} title={label} onClick={onClick} className="grid h-11 w-11 place-items-center rounded-full transition hover:bg-white/15 active:scale-90">
      {children}
    </button>
  );
}
