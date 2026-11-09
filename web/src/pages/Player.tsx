import { useQuery } from "@tanstack/react-query";
import clsx from "clsx";
import Hls from "hls.js";
import {
  ArrowLeft,
  Captions,
  Check,
  ChevronRight,
  Gauge,
  Maximize,
  Minimize,
  Pause,
  PictureInPicture2,
  Play,
  RotateCcw,
  RotateCw,
  Settings2,
  SkipForward,
  Volume1,
  Volume2,
  VolumeX,
} from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";
import { Img } from "../components/cards";
import { Button, Spinner } from "../components/ui";
import { api, landscapeImage, type Item, type PlaybackInfo } from "../lib/api";
import { episodeLabel, formatClock } from "../lib/format";
import { getPrefs, useLatest, usePrefs } from "../lib/hooks";
import { t } from "../lib/i18n";
import { canDirectPlay, qualities, type QualityId } from "../lib/playback";
import { activeCues, parseVTT, type Cue } from "../lib/vtt";

type Mode = "direct" | "hls";

interface Source {
  mode: Mode;
  offset: number; // absolute seconds at video.currentTime === 0
  url: string;
  sessionId?: string;
}

type Menu = null | "subs" | "settings" | "audio" | "quality" | "speed";

const SPEEDS = [0.5, 0.75, 1, 1.25, 1.5, 2];

export function Player() {
  const { id } = useParams();
  const [sp] = useSearchParams();
  const navigate = useNavigate();
  const [prefs, setPrefs] = usePrefs();

  const { data: info, error: infoError } = useQuery({
    queryKey: ["playback", id],
    queryFn: () => api.get<PlaybackInfo>(`/api/items/${id}/playback`),
    staleTime: 0,
    gcTime: 0,
  });
  const { data: nextEp } = useQuery({
    queryKey: ["next", id],
    queryFn: () => api.get<Item | null>(`/api/items/${id}/next`),
    enabled: info?.item.type === "Episode",
  });

  const videoRef = useRef<HTMLVideoElement>(null);
  const wrapRef = useRef<HTMLDivElement>(null);
  const hlsRef = useRef<Hls | null>(null);
  const [source, setSource] = useState<Source | null>(null);
  const sourceRef = useLatest(source);
  const [playing, setPlaying] = useState(false);
  const [waiting, setWaiting] = useState(true);
  const [time, setTime] = useState(0); // absolute
  const [buffered, setBuffered] = useState(0); // absolute
  const [mediaDuration, setMediaDuration] = useState(0);
  const [volume, setVolume] = useState(prefs.volume);
  const [muted, setMuted] = useState(prefs.muted);
  const [fullscreen, setFullscreen] = useState(false);
  const [controls, setControls] = useState(true);
  const [menu, setMenu] = useState<Menu>(null);
  const [error, setError] = useState<string | null>(null);
  const [subIndex, setSubIndex] = useState(-1);
  const [burnSub, setBurnSub] = useState<number | undefined>(undefined);
  const [cues, setCues] = useState<Cue[]>([]);
  const [audioIndex, setAudioIndex] = useState<number | undefined>(undefined);
  const [quality, setQuality] = useState<QualityId>("original");
  const [speed, setSpeed] = useState(1);
  const [flash, setFlash] = useState<null | "play" | "pause" | "back" | "fwd">(null);
  const [countdown, setCountdown] = useState<number | null>(null);
  const [hover, setHover] = useState<{ x: number; t: number } | null>(null);
  const [dismissedUpNext, setDismissedUpNext] = useState(false);
  const hideTimer = useRef<number | undefined>(undefined);
  const lastReport = useRef(0);

  const item = info?.item;
  const duration = info?.duration || item?.duration || mediaDuration || 0;
  const timeRef = useLatest(time);

  // ---- Source selection ----
  const startHls = useCallback(
    async (start: number, opts: { audio?: number; quality?: QualityId; burn?: number | null } = {}) => {
      if (!info) return;
      const q = qualities.find((x) => x.id === (opts.quality ?? quality))!;
      setWaiting(true);
      setError(null);
      try {
        const res = await api.post<{ url: string; start: number; sessionId: string }>(`/api/items/${info.item.id}/hls`, {
          start: Math.max(0, start),
          audio: opts.audio ?? audioIndex,
          maxHeight: q.maxHeight,
          maxBitrate: q.maxBitrate,
          burnSubtitle: (opts.burn === undefined ? burnSub : opts.burn) ?? undefined,
        });
        const prev = sourceRef.current;
        if (prev?.sessionId && prev.sessionId !== res.sessionId) api.del(`/api/hls/${prev.sessionId}`).catch(() => {});
        setSource({ mode: "hls", offset: res.start, url: res.url, sessionId: res.sessionId });
      } catch (e) {
        setError((e as Error).message);
        setWaiting(false);
      }
    },
    [info, quality, audioIndex, burnSub, sourceRef],
  );

  const initialStart = useMemo(() => {
    if (!info) return 0;
    const q = sp.get("t");
    if (q !== null) return parseFloat(q) || 0;
    return info.item.userData.played ? 0 : info.item.userData.position || 0;
  }, [info, sp]);

  useEffect(() => {
    if (!info) return;
    const p = getPrefs();
    // Default subtitle: forced/default track in preferred language, or preferred language if set.
    const subs = info.subtitles.filter((s) => s.url);
    const pref = p.subtitleLang;
    const pick =
      (pref && subs.find((s) => s.language?.startsWith(pref) && !s.forced)) ||
      subs.find((s) => s.forced && (!pref || s.language?.startsWith(pref))) ||
      subs.find((s) => s.default);
    setSubIndex(pick ? pick.index : -1);
    if (p.audioLang) {
      const a = info.audioTracks.find((x) => x.language?.startsWith(p.audioLang));
      if (a && !a.default && info.audioTracks.length > 1 && info.canTranscode) setAudioIndex(a.index);
    }
    const direct = canDirectPlay(info);
    if (direct.ok || !info.canTranscode) {
      setSource({ mode: "direct", offset: 0, url: info.directUrl });
    } else {
      startHls(initialStart);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [info]);

  // Attach the source to the video element.
  useEffect(() => {
    const v = videoRef.current;
    if (!v || !source) return;
    hlsRef.current?.destroy();
    hlsRef.current = null;
    const startRel = source.mode === "direct" ? (timeRef.current || initialStart) : 0;
    if (source.mode === "hls" && Hls.isSupported()) {
      const hls = new Hls({ startPosition: 0, maxBufferLength: 60, backBufferLength: 90, enableWorker: true, lowLatencyMode: false });
      hls.loadSource(source.url);
      hls.attachMedia(v);
      hls.on(Hls.Events.ERROR, (_, d) => {
        if (d.fatal) {
          if (d.type === Hls.ErrorTypes.NETWORK_ERROR) hls.startLoad();
          else if (d.type === Hls.ErrorTypes.MEDIA_ERROR) hls.recoverMediaError();
          else setError(d.details);
        }
      });
      hlsRef.current = hls;
    } else {
      v.src = source.url;
    }
    const onMeta = () => {
      if (source.mode === "direct" && startRel > 0) v.currentTime = startRel;
      v.play().catch(() => setPlaying(false));
    };
    v.addEventListener("loadedmetadata", onMeta, { once: true });
    return () => v.removeEventListener("loadedmetadata", onMeta);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [source]);

  // Cleanup on unmount: stop transcode + report stop.
  useEffect(() => {
    return () => {
      hlsRef.current?.destroy();
      const s = sourceRef.current;
      if (s?.sessionId) fetch(`/api/hls/${s.sessionId}`, { method: "DELETE", keepalive: true }).catch(() => {});
    };
  }, [sourceRef]);

  // ---- Progress reporting ----
  const report = useCallback(
    (event: "progress" | "stop" | "start", paused = false) => {
      if (!item) return;
      const body = JSON.stringify({ position: timeRef.current, duration, paused, event, method: sourceRef.current?.mode === "hls" ? "Transcode" : "DirectPlay" });
      fetch(`/api/items/${item.id}/progress`, { method: "POST", body, headers: { "Content-Type": "application/json" }, keepalive: true }).catch(() => {});
      lastReport.current = Date.now();
    },
    [item, duration, timeRef, sourceRef],
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

  // ---- Subtitles ----
  useEffect(() => {
    setCues([]);
    const track = info?.subtitles.find((s) => s.index === subIndex);
    if (!track?.url) return;
    let cancelled = false;
    fetch(track.url)
      .then((r) => (r.ok ? r.text() : Promise.reject(new Error(r.statusText))))
      .then((txt) => !cancelled && setCues(parseVTT(txt)))
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [subIndex, info]);
  const currentCues = useMemo(() => activeCues(cues, time), [cues, time]);

  // ---- Video events ----
  const onTimeUpdate = () => {
    const v = videoRef.current!;
    const abs = (source?.offset ?? 0) + v.currentTime;
    setTime(abs);
    if (v.buffered.length) setBuffered((source?.offset ?? 0) + v.buffered.end(v.buffered.length - 1));
    if (Date.now() - lastReport.current > 10_000 && !v.paused) report("progress");
  };

  const onVideoError = () => {
    const v = videoRef.current!;
    if (source?.mode === "direct" && info?.canTranscode) {
      startHls(timeRef.current || initialStart);
      return;
    }
    const code = v.error?.code;
    setError(code === 4 && !info?.canTranscode ? t("This file can't be played in the browser. Install ffmpeg on the server to enable transcoding.") : v.error?.message || t("Playback error"));
    setWaiting(false);
  };

  // ---- Controls visibility ----
  const poke = useCallback(() => {
    setControls(true);
    window.clearTimeout(hideTimer.current);
    hideTimer.current = window.setTimeout(() => {
      if (!videoRef.current?.paused) {
        setControls(false);
        setMenu(null);
      }
    }, 3200);
  }, []);
  useEffect(() => {
    poke();
    return () => window.clearTimeout(hideTimer.current);
  }, [poke]);

  // ---- Actions ----
  const togglePlay = useCallback(() => {
    const v = videoRef.current;
    if (!v) return;
    if (v.paused) {
      v.play().catch(() => {});
      setFlash("play");
    } else {
      v.pause();
      setFlash("pause");
      report("progress", true);
    }
    setTimeout(() => setFlash(null), 500);
  }, [report]);

  const seek = useCallback(
    (abs: number) => {
      const v = videoRef.current;
      if (!v || !source) return;
      abs = Math.max(0, duration ? Math.min(abs, duration - 1) : abs);
      setTime(abs);
      if (source.mode === "direct") {
        v.currentTime = abs;
        return;
      }
      const rel = abs - source.offset;
      const seekEnd = v.seekable.length ? v.seekable.end(v.seekable.length - 1) : 0;
      if (rel >= 0 && rel <= seekEnd - 1) v.currentTime = rel;
      else startHls(abs);
    },
    [source, duration, startHls],
  );

  const skip = useCallback(
    (d: number) => {
      seek(timeRef.current + d);
      setFlash(d < 0 ? "back" : "fwd");
      setTimeout(() => setFlash(null), 500);
    },
    [seek, timeRef],
  );

  const toggleFullscreen = useCallback(() => {
    if (document.fullscreenElement) document.exitFullscreen();
    else wrapRef.current?.requestFullscreen?.().catch(() => {});
  }, []);
  useEffect(() => {
    const on = () => setFullscreen(!!document.fullscreenElement);
    document.addEventListener("fullscreenchange", on);
    return () => document.removeEventListener("fullscreenchange", on);
  }, []);

  useEffect(() => {
    const v = videoRef.current;
    if (!v) return;
    v.volume = volume;
    v.muted = muted;
    setPrefs({ volume, muted });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [volume, muted]);
  useEffect(() => {
    if (videoRef.current) videoRef.current.playbackRate = speed;
  }, [speed, source]);

  const goNext = useCallback(() => {
    if (nextEp) navigate(`/play/${nextEp.id}?t=0`, { replace: true });
  }, [nextEp, navigate]);

  const exit = useCallback(() => {
    report("stop");
    if (window.history.length > 1) navigate(-1);
    else navigate(item ? `/item/${item.seriesId || item.id}` : "/");
  }, [navigate, item, report]);

  // Keyboard shortcuts
  const keys = useLatest({ togglePlay, skip, toggleFullscreen, goNext, exit });
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const k = keys.current;
      if (e.target instanceof HTMLInputElement && e.target.type !== "range") return;
      switch (e.key) {
        case " ":
        case "k":
          e.preventDefault();
          k.togglePlay();
          break;
        case "ArrowLeft":
        case "j":
          e.preventDefault();
          k.skip(-10);
          break;
        case "ArrowRight":
        case "l":
          e.preventDefault();
          k.skip(e.shiftKey ? 30 : 10);
          break;
        case "ArrowUp":
          e.preventDefault();
          setVolume((v) => Math.min(1, v + 0.05));
          setMuted(false);
          break;
        case "ArrowDown":
          e.preventDefault();
          setVolume((v) => Math.max(0, v - 0.05));
          break;
        case "f":
          k.toggleFullscreen();
          break;
        case "m":
          setMuted((m) => !m);
          break;
        case "c":
          setSubIndex((cur) => {
            const subs = info?.subtitles.filter((s) => s.url) ?? [];
            if (!subs.length) return -1;
            const i = subs.findIndex((s) => s.index === cur);
            return i + 1 < subs.length ? subs[i + 1].index : -1;
          });
          break;
        case "n":
          k.goNext();
          break;
        case "Escape":
          if (!document.fullscreenElement) k.exit();
          break;
        default:
          return;
      }
      poke();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [keys, poke, info]);

  // ---- Up next ----
  const remaining = duration ? duration - time : Infinity;
  const showUpNext = !!nextEp && !dismissedUpNext && remaining < 40 && duration > 120;
  useEffect(() => {
    if (countdown === null) return;
    if (countdown <= 0) {
      goNext();
      return;
    }
    const id = setTimeout(() => setCountdown((c) => (c === null ? null : c - 1)), 1000);
    return () => clearTimeout(id);
  }, [countdown, goNext]);

  const onEnded = () => {
    report("stop");
    if (nextEp && prefs.autoplayNext) setCountdown(8);
    else if (!nextEp) exit();
  };

  const changeAudio = (index: number) => {
    setAudioIndex(index);
    setMenu(null);
    if (info?.canTranscode) startHls(timeRef.current, { audio: index });
  };
  /** Goes back to direct play when nothing requires transcoding anymore. */
  const reconfigure = (q: QualityId, burn: number | null) => {
    if (q === "original" && burn === null && info && canDirectPlay(info).ok && audioIndex === undefined) {
      const s = sourceRef.current;
      if (s?.sessionId) api.del(`/api/hls/${s.sessionId}`).catch(() => {});
      setSource({ mode: "direct", offset: 0, url: info.directUrl });
    } else startHls(timeRef.current, { quality: q, burn });
  };
  const changeQuality = (q: QualityId) => {
    setQuality(q);
    setMenu(null);
    reconfigure(q, burnSub ?? null);
  };
  const selectSubtitle = (index: number) => {
    setMenu(null);
    setSubIndex(index);
    const track = info?.subtitles.find((s) => s.index === index);
    if (track?.burnIn) {
      setBurnSub(index);
      startHls(timeRef.current, { burn: index });
    } else if (burnSub !== undefined) {
      setBurnSub(undefined);
      reconfigure(quality, null);
    }
  };

  if (infoError)
    return (
      <div className="grid h-dvh place-items-center bg-black">
        <div className="text-center">
          <p className="mb-4 text-ink-300">{(infoError as Error).message}</p>
          <Button onClick={() => navigate("/")}>{t("Go home")}</Button>
        </div>
      </div>
    );

  const title = item?.type === "Episode" ? item.seriesName : item?.name;
  const subtitle = item?.type === "Episode" ? `${episodeLabel(item)} · ${item.name}` : item?.year ? String(item.year) : "";
  const pct = duration ? (time / duration) * 100 : 0;
  const bufPct = duration ? (buffered / duration) * 100 : 0;
  const VolumeIcon = muted || volume === 0 ? VolumeX : volume < 0.5 ? Volume1 : Volume2;
  const subSize = { small: "text-[clamp(16px,2.2vw,28px)]", medium: "text-[clamp(18px,2.8vw,38px)]", large: "text-[clamp(22px,3.6vw,50px)]" }[prefs.subtitleSize];

  return (
    <div
      ref={wrapRef}
      className={clsx("fixed inset-0 z-[90] bg-black select-none", !controls && "cursor-none")}
      onMouseMove={poke}
      onTouchStart={poke}
    >
      <video
        ref={videoRef}
        className="absolute inset-0 h-full w-full"
        playsInline
        onClick={() => {
          if (menu) setMenu(null);
          else togglePlay();
        }}
        onDoubleClick={toggleFullscreen}
        onPlay={() => {
          setPlaying(true);
          if (timeRef.current === 0 || lastReport.current === 0) report("start");
        }}
        onPause={() => {
          setPlaying(false);
          setControls(true);
        }}
        onWaiting={() => setWaiting(true)}
        onPlaying={() => setWaiting(false)}
        onCanPlay={() => setWaiting(false)}
        onTimeUpdate={onTimeUpdate}
        onDurationChange={(e) => isFinite(e.currentTarget.duration) && setMediaDuration(e.currentTarget.duration)}
        onEnded={onEnded}
        onError={onVideoError}
      />

      {/* Subtitles */}
      {currentCues.length > 0 && (
        <div className={clsx("pointer-events-none absolute inset-x-0 flex justify-center px-[8%] text-center transition-all duration-300", controls ? "bottom-[17%]" : "bottom-[7%]")}>
          <div className={clsx("font-sans leading-snug font-semibold text-white", subSize)} style={{ textShadow: "0 0 4px #000, 0 0 8px #000, 0 2px 3px #000" }}>
            {currentCues.map((c, i) => (
              <div key={i} dangerouslySetInnerHTML={{ __html: c.html }} />
            ))}
          </div>
        </div>
      )}

      {/* Center feedback */}
      {(waiting || !info) && !error && (
        <div className="pointer-events-none absolute inset-0 grid place-items-center">
          <Spinner className="h-14 w-14 border-[3px]" />
        </div>
      )}
      {flash && (
        <div className="pointer-events-none absolute inset-0 grid place-items-center">
          <div className="grid h-20 w-20 animate-scale-in place-items-center rounded-full bg-black/50 backdrop-blur">
            {flash === "play" && <Play size={34} fill="white" className="ml-1" />}
            {flash === "pause" && <Pause size={34} fill="white" />}
            {flash === "back" && <RotateCcw size={32} />}
            {flash === "fwd" && <RotateCw size={32} />}
          </div>
        </div>
      )}
      {error && (
        <div className="absolute inset-0 grid place-items-center bg-black/70 p-6">
          <div className="max-w-md text-center">
            <h2 className="mb-2 font-display text-2xl font-semibold">{t("Playback error")}</h2>
            <p className="mb-6 text-ink-300">{error}</p>
            <div className="flex justify-center gap-2">
              <Button onClick={exit}>{t("Back")}</Button>
              {info?.canTranscode && (
                <Button variant="primary" onClick={() => startHls(timeRef.current || initialStart)}>
                  <RotateCw size={16} /> {t("Transcoding")}
                </Button>
              )}
            </div>
          </div>
        </div>
      )}

      {/* Top bar */}
      <div
        className={clsx(
          "absolute inset-x-0 top-0 flex items-start gap-4 bg-gradient-to-b from-black/80 to-transparent px-4 pt-4 pb-16 transition-opacity duration-300 sm:px-8 sm:pt-6",
          controls ? "opacity-100" : "pointer-events-none opacity-0",
        )}
      >
        <button onClick={exit} className="focus-ring grid h-11 w-11 shrink-0 place-items-center rounded-full bg-white/10 backdrop-blur transition hover:bg-white/20" aria-label={t("Back")}>
          <ArrowLeft size={22} />
        </button>
        <div className="min-w-0 pt-0.5">
          <div className="truncate font-display text-xl font-semibold sm:text-2xl">{title}</div>
          {subtitle && <div className="truncate text-sm text-ink-300">{subtitle}</div>}
        </div>
        <div className="flex-1" />
        {source && (
          <span className="mt-2 hidden rounded-full bg-white/10 px-3 py-1 text-[11px] font-semibold tracking-wide text-ink-200 uppercase backdrop-blur sm:block">
            {source.mode === "hls" ? t("Transcoding") : t("Direct play")}
          </span>
        )}
      </div>

      {/* Up next */}
      {(showUpNext || countdown !== null) && nextEp && (
        <div className="absolute right-4 bottom-32 w-[min(360px,calc(100%-2rem))] animate-fade-up overflow-hidden rounded-2xl bg-ink-900/90 shadow-2xl ring-1 ring-white/15 backdrop-blur-xl sm:right-8">
          <div className="relative aspect-video">
            <Img src={landscapeImage(nextEp, 640)} className="h-full w-full object-cover" fallback={<div className="h-full w-full bg-ink-800" />} />
            <div className="absolute inset-0 bg-gradient-to-t from-ink-900 to-transparent" />
            <div className="absolute bottom-3 left-4 right-4">
              <div className="text-[11px] font-semibold tracking-wider text-accent-400 uppercase">{t("Up next")}</div>
              <div className="truncate font-semibold">
                {episodeLabel(nextEp)} · {nextEp.name}
              </div>
            </div>
          </div>
          <div className="flex gap-2 p-3">
            <Button variant="primary" size="sm" className="flex-1" onClick={goNext}>
              <SkipForward size={15} fill="currentColor" /> {countdown !== null ? t("Playing in {0}s", countdown) : t("Play now")}
            </Button>
            <Button
              size="sm"
              onClick={() => {
                setDismissedUpNext(true);
                setCountdown(null);
              }}
            >
              {t("Watch credits")}
            </Button>
          </div>
        </div>
      )}

      {/* Bottom controls */}
      <div
        className={clsx(
          "absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/90 via-black/50 to-transparent px-4 pt-24 pb-4 transition-opacity duration-300 sm:px-8 sm:pb-6",
          controls ? "opacity-100" : "pointer-events-none opacity-0",
        )}
      >
        {/* Scrubber */}
        <div
          className="group relative mb-3 flex h-5 cursor-pointer items-center"
          onMouseMove={(e) => {
            const r = e.currentTarget.getBoundingClientRect();
            const x = Math.min(Math.max(0, e.clientX - r.left), r.width);
            setHover({ x, t: (x / r.width) * duration });
          }}
          onMouseLeave={() => setHover(null)}
          onClick={(e) => {
            const r = e.currentTarget.getBoundingClientRect();
            seek(((e.clientX - r.left) / r.width) * duration);
          }}
        >
          <div className="relative h-1 w-full overflow-hidden rounded-full bg-white/20 transition-all group-hover:h-1.5">
            <div className="absolute inset-y-0 left-0 bg-white/30" style={{ width: `${bufPct}%` }} />
            <div className="absolute inset-y-0 left-0 bg-accent-400" style={{ width: `${pct}%` }} />
          </div>
          {info?.chapters?.map((c, i) =>
            c.start > 0 && duration ? <span key={i} className="absolute h-1.5 w-0.5 bg-black/70" style={{ left: `${(c.start / duration) * 100}%` }} /> : null,
          )}
          <div className="absolute h-4 w-4 -translate-x-1/2 scale-0 rounded-full bg-white shadow-lg transition group-hover:scale-100" style={{ left: `${pct}%` }} />
          {hover && duration > 0 && (
            <div className="pointer-events-none absolute bottom-6 -translate-x-1/2 rounded-lg bg-black/85 px-2.5 py-1 text-xs font-semibold tabular-nums" style={{ left: hover.x }}>
              {formatClock(hover.t)}
              {info?.chapters?.length ? (
                <div className="max-w-40 truncate font-normal text-ink-300">{[...info.chapters].reverse().find((c) => c.start <= hover.t)?.title}</div>
              ) : null}
            </div>
          )}
        </div>

        <div className="flex items-center gap-1 sm:gap-2">
          <CtrlButton label={playing ? "Pause" : t("Play")} onClick={togglePlay}>
            {playing ? <Pause size={26} fill="white" /> : <Play size={26} fill="white" className="ml-0.5" />}
          </CtrlButton>
          <CtrlButton label="-10s" onClick={() => skip(-10)}>
            <RotateCcw size={22} />
          </CtrlButton>
          <CtrlButton label="+10s" onClick={() => skip(10)}>
            <RotateCw size={22} />
          </CtrlButton>
          <div className="group/vol flex items-center">
            <CtrlButton label="Mute" onClick={() => setMuted((m) => !m)}>
              <VolumeIcon size={23} />
            </CtrlButton>
            <input
              type="range"
              min={0}
              max={1}
              step={0.01}
              value={muted ? 0 : volume}
              onChange={(e) => {
                setVolume(parseFloat(e.target.value));
                setMuted(false);
              }}
              className="slider hidden w-0 opacity-0 transition-all duration-300 group-hover/vol:w-24 group-hover/vol:opacity-100 sm:block"
              style={{ ["--value" as string]: `${(muted ? 0 : volume) * 100}%` }}
            />
          </div>
          <div className="ml-2 text-[13px] font-medium text-ink-200 tabular-nums sm:text-sm">
            {formatClock(time)} <span className="text-ink-500">/</span> {formatClock(duration)}
          </div>
          <div className="flex-1" />
          {nextEp && (
            <CtrlButton label={t("Next")} onClick={goNext}>
              <SkipForward size={22} />
            </CtrlButton>
          )}
          <div className="relative">
            <CtrlButton label={t("Subtitles")} onClick={() => setMenu(menu === "subs" ? null : "subs")} active={subIndex >= 0}>
              <Captions size={23} />
            </CtrlButton>
            {menu === "subs" && info && (
              <MenuPanel title={t("Subtitles")}>
                <MenuItem active={subIndex === -1} onClick={() => selectSubtitle(-1)}>
                  {t("Off")}
                </MenuItem>
                {info.subtitles.map((s) => (
                  <MenuItem
                    key={s.index}
                    active={subIndex === s.index}
                    disabled={!s.url && !(s.burnIn && info.canTranscode)}
                    onClick={() => selectSubtitle(s.index)}
                  >
                    {s.label}
                    {s.burnIn && <span className="ml-1.5 rounded bg-white/10 px-1.5 py-px text-[10px] font-semibold text-ink-300 uppercase">{s.codec === "hdmv_pgs_subtitle" ? "PGS" : "VobSub"}</span>}
                  </MenuItem>
                ))}
                <div className="mt-1 flex gap-1 border-t border-white/10 px-2 pt-2">
                  {(["small", "medium", "large"] as const).map((sz) => (
                    <button
                      key={sz}
                      onClick={() => setPrefs({ subtitleSize: sz })}
                      className={clsx("flex-1 rounded-lg py-1.5 text-xs font-semibold", prefs.subtitleSize === sz ? "bg-white text-ink-950" : "bg-white/10")}
                    >
                      {t(sz === "small" ? "Small" : sz === "medium" ? "Medium" : "Large")}
                    </button>
                  ))}
                </div>
              </MenuPanel>
            )}
          </div>
          <div className="relative">
            <CtrlButton label={t("Settings")} onClick={() => setMenu(menu && menu !== "subs" ? null : "settings")}>
              <Settings2 size={22} />
            </CtrlButton>
            {menu === "settings" && (
              <MenuPanel title={t("Settings")}>
                {info && info.audioTracks.length > 0 && (
                  <MenuNav onClick={() => setMenu("audio")} value={info.audioTracks.find((a) => a.index === audioIndex)?.label ?? info.audioTracks.find((a) => a.default)?.label ?? info.audioTracks[0].label}>
                    {t("Audio")}
                  </MenuNav>
                )}
                {info?.canTranscode && (
                  <MenuNav onClick={() => setMenu("quality")} value={quality === "original" ? t("Original") : qualities.find((q) => q.id === quality)!.label}>
                    {t("Quality")}
                  </MenuNav>
                )}
                <MenuNav onClick={() => setMenu("speed")} value={speed === 1 ? t("Normal") : `${speed}×`}>
                  {t("Speed")}
                </MenuNav>
              </MenuPanel>
            )}
            {menu === "audio" && info && (
              <MenuPanel title={t("Audio")} onBack={() => setMenu("settings")}>
                {info.audioTracks.map((a) => (
                  <MenuItem
                    key={a.index}
                    active={audioIndex === undefined ? !!a.default || info.audioTracks[0].index === a.index : audioIndex === a.index}
                    disabled={!info.canTranscode && info.audioTracks.length > 1}
                    onClick={() => changeAudio(a.index)}
                  >
                    {a.label}
                  </MenuItem>
                ))}
              </MenuPanel>
            )}
            {menu === "quality" && (
              <MenuPanel title={t("Quality")} onBack={() => setMenu("settings")}>
                {qualities.map((q) => (
                  <MenuItem key={q.id} active={quality === q.id} onClick={() => changeQuality(q.id)}>
                    {q.id === "original" ? t("Original") : q.label}
                  </MenuItem>
                ))}
              </MenuPanel>
            )}
            {menu === "speed" && (
              <MenuPanel title={t("Speed")} onBack={() => setMenu("settings")}>
                {SPEEDS.map((s) => (
                  <MenuItem key={s} active={speed === s} onClick={() => (setSpeed(s), setMenu(null))}>
                    <Gauge size={15} className="mr-2 inline text-ink-400" />
                    {s === 1 ? t("Normal") : `${s}×`}
                  </MenuItem>
                ))}
              </MenuPanel>
            )}
          </div>
          {document.pictureInPictureEnabled && (
            <CtrlButton label="Picture in picture" onClick={() => videoRef.current?.requestPictureInPicture().catch(() => {})}>
              <PictureInPicture2 size={22} />
            </CtrlButton>
          )}
          <CtrlButton label="Fullscreen" onClick={toggleFullscreen}>
            {fullscreen ? <Minimize size={22} /> : <Maximize size={22} />}
          </CtrlButton>
        </div>
      </div>
    </div>
  );
}

function CtrlButton({ label, onClick, children, active }: { label: string; onClick: () => void; children: React.ReactNode; active?: boolean }) {
  return (
    <button
      aria-label={label}
      title={label}
      onClick={(e) => {
        e.stopPropagation();
        onClick();
      }}
      className={clsx("focus-ring relative grid h-11 w-11 place-items-center rounded-full transition hover:bg-white/15 active:scale-90", active && "text-accent-400")}
    >
      {children}
      {active && <span className="absolute bottom-1.5 h-0.5 w-4 rounded-full bg-accent-400" />}
    </button>
  );
}

function MenuPanel({ title, children, onBack }: { title: string; children: React.ReactNode; onBack?: () => void }) {
  return (
    <div
      className="absolute right-0 bottom-14 max-h-[60vh] w-72 animate-scale-in overflow-y-auto rounded-2xl bg-ink-900/95 p-1.5 shadow-2xl ring-1 ring-white/15 backdrop-blur-xl"
      onClick={(e) => e.stopPropagation()}
    >
      <div className="flex items-center gap-2 px-3 pt-2 pb-2 text-[12px] font-semibold tracking-wider text-ink-400 uppercase">
        {onBack && (
          <button onClick={onBack} className="-ml-1 rounded-full p-1 hover:bg-white/10 hover:text-white">
            <ArrowLeft size={15} />
          </button>
        )}
        {title}
      </div>
      {children}
    </div>
  );
}

function MenuItem({ active, disabled, onClick, children }: { active?: boolean; disabled?: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button
      disabled={disabled}
      onClick={onClick}
      className="flex w-full items-center gap-2 rounded-xl px-3 py-2.5 text-left text-sm transition hover:bg-white/10 disabled:opacity-40 disabled:hover:bg-transparent"
    >
      <span className="w-4 shrink-0">{active && <Check size={16} className="text-accent-400" />}</span>
      <span className="min-w-0 flex-1 truncate">{children}</span>
    </button>
  );
}

function MenuNav({ onClick, value, children }: { onClick: () => void; value: string; children: React.ReactNode }) {
  return (
    <button onClick={onClick} className="flex w-full items-center gap-3 rounded-xl px-3 py-2.5 text-left text-sm transition hover:bg-white/10">
      <span className="flex-1">{children}</span>
      <span className="max-w-32 truncate text-ink-400">{value}</span>
      <ChevronRight size={16} className="text-ink-500" />
    </button>
  );
}
