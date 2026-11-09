import { useQueryClient } from "@tanstack/react-query";
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { api, posterImage, type Item } from "./api";
import { getPrefs } from "./hooks";

export interface Track {
  id: string;
  name: string;
  artist: string;
  album?: string;
  albumId?: string;
  image?: string;
  duration: number;
  container?: string;
  /** For audiobooks: the book id and the chapter start offset in the book. */
  bookId?: string;
  bookOffset?: number;
}

type Repeat = "off" | "all" | "one";

interface AudioState {
  queue: Track[];
  index: number;
  playing: boolean;
  time: number;
  duration: number;
  buffering: boolean;
  shuffle: boolean;
  repeat: Repeat;
  volume: number;
  current?: Track;
  playTracks: (tracks: Track[], start?: number, startAt?: number) => void;
  playItem: (item: Item) => Promise<void>;
  toggle: () => void;
  next: () => void;
  prev: () => void;
  seek: (t: number) => void;
  jump: (i: number) => void;
  setShuffle: (v: boolean) => void;
  setRepeat: (r: Repeat) => void;
  setVolume: (v: number) => void;
  enqueue: (tracks: Track[]) => void;
  removeAt: (i: number) => void;
  close: () => void;
}

const Ctx = createContext<AudioState | null>(null);

export function useAudio() {
  const c = useContext(Ctx);
  if (!c) throw new Error("useAudio outside provider");
  return c;
}

const nativeTypes: Record<string, string> = {
  mp3: "audio/mpeg",
  m4a: "audio/mp4",
  m4b: "audio/mp4",
  aac: "audio/aac",
  flac: "audio/flac",
  ogg: "audio/ogg",
  oga: "audio/ogg",
  opus: 'audio/ogg; codecs="opus"',
  wav: "audio/wav",
  webm: "audio/webm",
};

function canPlayNative(container?: string) {
  const type = container ? nativeTypes[container.toLowerCase()] : undefined;
  if (!type) return false;
  return document.createElement("audio").canPlayType(type) !== "";
}

export function trackFromItem(it: Item, album?: Item, book?: Item, bookOffset?: number): Track {
  const image = posterImage(album ?? book ?? it, 400) ?? (it.albumId ? `/api/items/${it.albumId}/images/primary?w=400` : undefined);
  return {
    id: it.id,
    name: it.name,
    artist: (it.artists ?? []).join(", ") || it.albumArtist || book?.artists?.join(", ") || "",
    album: album?.name ?? book?.name,
    albumId: album?.id ?? it.albumId,
    image,
    duration: it.duration ?? 0,
    container: it.container,
    bookId: book?.id,
    bookOffset,
  };
}

export function AudioProvider({ children }: { children: ReactNode }) {
  const qc = useQueryClient();
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const [queue, setQueue] = useState<Track[]>([]);
  const [order, setOrder] = useState<number[]>([]); // play order (indexes into queue)
  const [pos, setPos] = useState(0); // position in order
  const [playing, setPlaying] = useState(false);
  const [time, setTime] = useState(0);
  const [buffering, setBuffering] = useState(false);
  const [shuffle, setShuffleState] = useState(false);
  const [repeat, setRepeat] = useState<Repeat>("off");
  const [volume, setVolumeState] = useState(getPrefs().volume);
  const offsetRef = useRef(0); // seek offset for transcoded streams
  const lastReport = useRef(0);
  const pendingStart = useRef(0);

  if (!audioRef.current && typeof Audio !== "undefined") {
    audioRef.current = new Audio();
    audioRef.current.preload = "auto";
  }

  const index = order[pos] ?? 0;
  const current = queue[index];

  const report = useCallback(
    (t: Track | undefined, position: number, event: "progress" | "stop" = "progress") => {
      if (!t) return;
      const id = t.bookId ?? t.id;
      const abs = (t.bookOffset ?? 0) + position;
      fetch(`/api/items/${id}/progress`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ position: abs, event, method: "DirectPlay" }),
        keepalive: true,
      }).catch(() => {});
      lastReport.current = Date.now();
    },
    [],
  );

  const load = useCallback((t: Track, startAt = 0, autoplay = true) => {
    const a = audioRef.current!;
    offsetRef.current = 0;
    if (canPlayNative(t.container)) {
      a.src = `/api/items/${t.id}/file`;
      pendingStart.current = startAt;
    } else {
      offsetRef.current = startAt;
      a.src = `/api/items/${t.id}/audio?start=${startAt}`;
      pendingStart.current = 0;
    }
    setTime(startAt);
    if (autoplay) a.play().catch(() => setPlaying(false));
  }, []);

  // Load the current track whenever it changes.
  const currentId = current ? `${current.id}:${index}` : "";
  const startAtRef = useRef(0);
  useEffect(() => {
    if (!current) return;
    load(current, startAtRef.current);
    startAtRef.current = 0;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [currentId]);

  const buildOrder = useCallback((len: number, first: number, shuffled: boolean) => {
    const idx = Array.from({ length: len }, (_, i) => i);
    if (!shuffled) return idx;
    const rest = idx.filter((i) => i !== first);
    for (let i = rest.length - 1; i > 0; i--) {
      const j = Math.floor(Math.random() * (i + 1));
      [rest[i], rest[j]] = [rest[j], rest[i]];
    }
    return [first, ...rest];
  }, []);

  const playTracks = useCallback(
    (tracks: Track[], start = 0, startAt = 0) => {
      if (!tracks.length) return;
      report(current, time, "stop");
      startAtRef.current = startAt;
      const o = buildOrder(tracks.length, start, shuffle);
      setQueue(tracks);
      setOrder(o);
      setPos(shuffle ? 0 : start);
      // Force reload even if the same track id is requested again.
      if (current && tracks[start]?.id === current.id) load(tracks[start], startAt);
    },
    [buildOrder, shuffle, current, time, load, report],
  );

  const playItem = useCallback(
    async (item: Item) => {
      if (item.type === "Audio") {
        const album = item.albumId ? await api.get<Item>(`/api/items/${item.albumId}`).catch(() => undefined) : undefined;
        if (album?.type === "AudioBook") {
          await playItem(album);
          return;
        }
        if (album) {
          const tracks = await api.get<Item[]>(`/api/items/${album.id}/children`);
          playTracks(tracks.map((t) => trackFromItem(t, album)), Math.max(0, tracks.findIndex((t) => t.id === item.id)));
        } else playTracks([trackFromItem(item)]);
        return;
      }
      if (item.type === "MusicAlbum") {
        const tracks = await api.get<Item[]>(`/api/items/${item.id}/children`);
        playTracks(tracks.map((t) => trackFromItem(t, item)));
        return;
      }
      if (item.type === "MusicArtist") {
        const r = await api.get<{ items: Item[] }>(`/api/items?type=Audio&artistId=${item.id}&sort=random&limit=200`);
        playTracks(r.items.map((t) => trackFromItem(t)));
        return;
      }
      if (item.type === "AudioBook") {
        const chapters = await api.get<Item[]>(`/api/items/${item.id}/children`);
        let offset = 0;
        const tracks = chapters.map((c) => {
          const t = trackFromItem(c, undefined, item, offset);
          offset += c.duration ?? 0;
          return t;
        });
        // Resume from the saved position.
        const resume = item.userData.played ? 0 : item.userData.position || 0;
        let start = 0;
        for (let i = 0; i < tracks.length; i++) if ((tracks[i].bookOffset ?? 0) <= resume) start = i;
        const within = Math.max(0, resume - (tracks[start]?.bookOffset ?? 0));
        setShuffleState(false);
        playTracks(tracks, start, within);
      }
    },
    [playTracks],
  );

  const goto = useCallback(
    (p: number) => {
      if (p < 0 || p >= order.length) return false;
      report(current, time, "stop");
      setPos(p);
      return true;
    },
    [order.length, current, time, report],
  );

  const next = useCallback(() => {
    if (!goto(pos + 1) && repeat === "all") goto(0);
  }, [goto, pos, repeat]);

  const prev = useCallback(() => {
    const a = audioRef.current!;
    if (offsetRef.current + a.currentTime > 4 || pos === 0) {
      seekRef.current(0);
      return;
    }
    goto(pos - 1);
  }, [goto, pos]);

  const seek = useCallback(
    (t: number) => {
      const a = audioRef.current!;
      if (!current) return;
      if (offsetRef.current > 0 || !canPlayNative(current.container)) {
        load(current, Math.max(0, t), !a.paused);
        return;
      }
      a.currentTime = Math.max(0, t);
      setTime(t);
    },
    [current, load],
  );
  const seekRef = useRef(seek);
  seekRef.current = seek;

  const toggle = useCallback(() => {
    const a = audioRef.current!;
    if (a.paused) a.play().catch(() => {});
    else {
      a.pause();
      report(current, offsetRef.current + a.currentTime);
    }
  }, [current, report]);

  // Element events
  useEffect(() => {
    const a = audioRef.current!;
    const onTime = () => {
      const t = offsetRef.current + a.currentTime;
      setTime(t);
      if (current?.bookId && Date.now() - lastReport.current > 15000 && !a.paused) report(current, t);
    };
    const onMeta = () => {
      if (pendingStart.current > 0) {
        a.currentTime = pendingStart.current;
        pendingStart.current = 0;
      }
    };
    const onEnded = () => {
      report(current, current?.duration || a.duration || 0, "stop");
      if (repeat === "one") {
        seekRef.current(0);
        a.play().catch(() => {});
        return;
      }
      if (pos + 1 < order.length) setPos(pos + 1);
      else if (repeat === "all") setPos(0);
      else {
        setPlaying(false);
        qc.invalidateQueries({ queryKey: ["home"] });
      }
    };
    const onPlay = () => setPlaying(true);
    const onPause = () => setPlaying(false);
    const onWaiting = () => setBuffering(true);
    const onPlaying = () => setBuffering(false);
    a.addEventListener("timeupdate", onTime);
    a.addEventListener("loadedmetadata", onMeta);
    a.addEventListener("ended", onEnded);
    a.addEventListener("play", onPlay);
    a.addEventListener("pause", onPause);
    a.addEventListener("waiting", onWaiting);
    a.addEventListener("playing", onPlaying);
    return () => {
      a.removeEventListener("timeupdate", onTime);
      a.removeEventListener("loadedmetadata", onMeta);
      a.removeEventListener("ended", onEnded);
      a.removeEventListener("play", onPlay);
      a.removeEventListener("pause", onPause);
      a.removeEventListener("waiting", onWaiting);
      a.removeEventListener("playing", onPlaying);
    };
  }, [current, pos, order.length, repeat, report, qc]);

  useEffect(() => {
    audioRef.current!.volume = volume;
  }, [volume]);

  // Media Session (lock screen / media keys)
  useEffect(() => {
    if (!("mediaSession" in navigator) || !current) return;
    navigator.mediaSession.metadata = new MediaMetadata({
      title: current.name,
      artist: current.artist,
      album: current.album ?? "",
      artwork: current.image ? [{ src: current.image, sizes: "400x400" }] : [],
    });
    navigator.mediaSession.setActionHandler("play", () => audioRef.current?.play());
    navigator.mediaSession.setActionHandler("pause", () => audioRef.current?.pause());
    navigator.mediaSession.setActionHandler("nexttrack", () => next());
    navigator.mediaSession.setActionHandler("previoustrack", () => prev());
    navigator.mediaSession.setActionHandler("seekto", (d) => d.seekTime != null && seek(d.seekTime));
  }, [current, next, prev, seek]);

  useEffect(() => {
    const onHide = () => current && report(current, time, "stop");
    window.addEventListener("pagehide", onHide);
    return () => window.removeEventListener("pagehide", onHide);
  }, [current, time, report]);

  const value = useMemo<AudioState>(
    () => ({
      queue,
      index,
      playing,
      time,
      duration: current?.duration || audioRef.current?.duration || 0,
      buffering,
      shuffle,
      repeat,
      volume,
      current,
      playTracks,
      playItem,
      toggle,
      next,
      prev,
      seek,
      jump: (i: number) => {
        const p = order.indexOf(i);
        if (p >= 0) goto(p);
      },
      setShuffle: (v: boolean) => {
        setShuffleState(v);
        const o = buildOrder(queue.length, index, v);
        setOrder(o);
        setPos(v ? 0 : index);
      },
      setRepeat,
      setVolume: (v: number) => setVolumeState(v),
      enqueue: (tracks: Track[]) => {
        if (!queue.length) {
          playTracks(tracks);
          return;
        }
        setQueue((q) => [...q, ...tracks]);
        setOrder((o) => [...o, ...tracks.map((_, i) => queue.length + i)]);
      },
      removeAt: (i: number) => {
        if (i === index) return;
        setQueue((q) => q.filter((_, j) => j !== i));
        setOrder((o) => o.filter((x) => x !== i).map((x) => (x > i ? x - 1 : x)));
        if (i < index) setPos((p) => p - 1);
      },
      close: () => {
        report(current, time, "stop");
        audioRef.current?.pause();
        audioRef.current!.removeAttribute("src");
        setQueue([]);
        setOrder([]);
        setPos(0);
      },
    }),
    [queue, index, playing, time, buffering, shuffle, repeat, volume, current, playTracks, playItem, toggle, next, prev, seek, order, goto, buildOrder, report],
  );

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}
