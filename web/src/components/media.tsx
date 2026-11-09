import clsx from "clsx";
import { BookOpen, ChevronLeft, ChevronRight, Disc3, Download, Headphones, Images, Music2, Pause, Play, Timer, User as UserIcon, X } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { Link, useNavigate } from "react-router-dom";
import { imageUrl, posterImage, type Item } from "../lib/api";
import { trackFromItem, useAudio } from "../lib/audio";
import { formatClock, formatDuration } from "../lib/format";
import { t, locale } from "../lib/i18n";
import { Img } from "./cards";

/** Returns a function that "plays" any item in the most appropriate way. */
export function usePlay() {
  const navigate = useNavigate();
  const audio = useAudio();
  return useCallback(
    (item: Item) => {
      switch (item.type) {
        case "Movie":
        case "Episode":
          navigate(`/play/${item.id}`);
          break;
        case "Series":
          navigate(`/item/${item.id}?autoplay=1`);
          break;
        case "Book":
          navigate(`/read/${item.id}`);
          break;
        default:
          audio.playItem(item);
      }
    },
    [navigate, audio],
  );
}

const iconFor = (type: string) =>
  ({ MusicAlbum: Disc3, MusicArtist: UserIcon, AudioBook: Headphones, PhotoAlbum: Images, Book: BookOpen, Audio: Music2 })[type] ?? Music2;

function MediaPlaceholder({ item }: { item: Item }) {
  const Icon = iconFor(item.type);
  const hue = [...item.name].reduce((a, c) => a + c.charCodeAt(0), 0) % 360;
  return (
    <div
      className="absolute inset-0 flex flex-col items-center justify-center gap-2 p-4 text-center"
      style={{ background: `linear-gradient(140deg, hsl(${hue} 35% 24%), hsl(${(hue + 60) % 360} 30% 10%))` }}
    >
      <Icon size={30} strokeWidth={1.4} className="text-white/35" />
      <span className="line-clamp-3 font-display text-[14px] font-semibold text-white/80">{item.name}</span>
    </div>
  );
}

/** Square card for albums, artists, audiobooks and photo albums; 2:3 for books. */
export function MediaCard({ item, sub }: { item: Item; sub?: string }) {
  const play = usePlay();
  const round = item.type === "MusicArtist";
  const portrait = item.type === "Book";
  const img = item.type === "MusicArtist" && !item.images.primary && item.images.thumb ? imageUrl(item.id, "thumb", item.images.thumb, 360) : posterImage(item, 360);
  const playable = item.type !== "PhotoAlbum";
  const pct = item.duration && item.userData.position ? Math.min(100, (item.userData.position / item.duration) * 100) : 0;
  const subtitle =
    sub ??
    (item.type === "MusicAlbum"
      ? [item.albumArtist, item.year].filter(Boolean).join(" · ")
      : item.type === "MusicArtist"
        ? t("{0} albums", item.childCount ?? 0)
        : item.type === "PhotoAlbum"
          ? t("{0} photos", item.childCount ?? 0)
          : (item.artists ?? []).join(", ") || (item.year ? String(item.year) : ""));
  return (
    <Link to={`/item/${item.id}`} className="group focus-ring block rounded-2xl" draggable={false}>
      <div
        className={clsx(
          "relative overflow-hidden bg-ink-800 shadow-lg shadow-black/40 ring-1 ring-white/[0.07] transition duration-300 ease-out group-hover:-translate-y-1 group-hover:shadow-2xl group-hover:ring-white/25",
          round ? "aspect-square rounded-full" : portrait ? "aspect-[2/3] rounded-2xl" : "aspect-square rounded-2xl",
        )}
      >
        <Img src={img} alt={item.name} className="absolute inset-0 h-full w-full object-cover" fallback={<MediaPlaceholder item={item} />} />
        {playable && (
          <button
            onClick={(e) => {
              e.preventDefault();
              play(item);
            }}
            aria-label={t("Play")}
            className={clsx(
              "absolute grid h-12 w-12 translate-y-2 place-items-center rounded-full bg-accent-400 text-ink-950 opacity-0 shadow-xl transition duration-300 group-hover:translate-y-0 group-hover:opacity-100 hover:scale-105",
              round ? "inset-0 m-auto" : "right-3 bottom-3",
            )}
          >
            {item.type === "Book" ? <BookOpen size={19} /> : <Play size={20} fill="currentColor" className="ml-0.5" />}
          </button>
        )}
        {pct > 0 && !item.userData.played && (
          <div className="absolute inset-x-3 bottom-3 h-1 overflow-hidden rounded-full bg-white/25">
            <div className="h-full bg-accent-400" style={{ width: `${pct}%` }} />
          </div>
        )}
      </div>
      <div className={clsx("mt-2.5 px-0.5", round && "text-center")}>
        <div className="truncate text-[14px] font-medium text-ink-100 group-hover:text-white">{item.name}</div>
        {subtitle && <div className="truncate text-[13px] text-ink-400">{subtitle}</div>}
      </div>
    </Link>
  );
}

/** Track list for albums, playlists, search results and audiobook chapters. */
export function TrackList({ tracks, album, showAlbum, book }: { tracks: Item[]; album?: Item; showAlbum?: boolean; book?: Item }) {
  const audio = useAudio();
  const discs = new Set(tracks.map((t) => t.discNumber ?? 1));
  const multiDisc = discs.size > 1;
  let offset = 0;
  const offsets = tracks.map((tr) => {
    const o = offset;
    offset += tr.duration ?? 0;
    return o;
  });
  const play = (i: number) => {
    if (book) {
      audio.playTracks(
        tracks.map((tr, j) => trackFromItem(tr, undefined, book, offsets[j])),
        i,
      );
    } else audio.playTracks(tracks.map((tr) => trackFromItem(tr, album)), i);
  };
  let lastDisc = -1;
  return (
    <div className="space-y-0.5">
      {tracks.map((tr, i) => {
        const isCurrent = audio.current?.id === tr.id;
        const disc = tr.discNumber ?? 1;
        const header = multiDisc && disc !== lastDisc;
        lastDisc = disc;
        return (
          <div key={tr.id}>
            {header && (
              <div className="flex items-center gap-2 px-3 pt-5 pb-2 text-[12px] font-semibold tracking-wider text-ink-400 uppercase">
                <Disc3 size={14} /> {t("Disc {0}", disc)}
              </div>
            )}
            <button
              onClick={() => (isCurrent ? audio.toggle() : play(i))}
              className={clsx("group grid w-full grid-cols-[2.5rem_1fr_auto] items-center gap-3 rounded-xl px-3 py-2.5 text-left transition", isCurrent ? "bg-white/[0.08]" : "hover:bg-white/[0.05]")}
            >
              <span className="grid place-items-center text-[14px] text-ink-400 tabular-nums">
                {isCurrent && audio.playing ? (
                  <Pause size={16} className="text-accent-400" fill="currentColor" />
                ) : (
                  <>
                    <span className={clsx("group-hover:hidden", isCurrent && "text-accent-400")}>{tr.trackNumber ?? i + 1}</span>
                    <Play size={16} fill="currentColor" className="hidden text-white group-hover:block" />
                  </>
                )}
              </span>
              <span className="min-w-0">
                <span className={clsx("block truncate text-[15px]", isCurrent ? "font-semibold text-accent-300" : "text-white")}>{tr.name}</span>
                {(showAlbum || (tr.artists && album && tr.artists.join() !== album.albumArtist)) && (
                  <span className="block truncate text-[13px] text-ink-400">
                    {[(tr.artists ?? []).join(", "), showAlbum ? tr.albumArtist : undefined].filter(Boolean).join(" · ")}
                  </span>
                )}
              </span>
              <span className="text-[13px] text-ink-400 tabular-nums">{tr.duration ? formatClock(tr.duration) : ""}</span>
            </button>
          </div>
        );
      })}
    </div>
  );
}

/** Header used by album, artist, audiobook and book pages. */
export function MediaHeader({
  item,
  image,
  round,
  portrait,
  kicker,
  meta,
  actions,
}: {
  item: Item;
  image?: string;
  round?: boolean;
  portrait?: boolean;
  kicker: string;
  meta: React.ReactNode;
  actions: React.ReactNode;
}) {
  return (
    <div className="relative overflow-hidden">
      <div className="absolute inset-0 -z-0">
        {image && <img src={image} alt="" className="h-full w-full scale-125 object-cover opacity-50 blur-3xl saturate-150" />}
        <div className="absolute inset-0 bg-gradient-to-b from-ink-950/30 via-ink-950/70 to-ink-950" />
      </div>
      <div className="relative flex flex-col items-center gap-8 px-5 pt-28 pb-10 text-center sm:flex-row sm:items-end sm:px-10 sm:text-left lg:px-14">
        <div
          className={clsx(
            "relative w-52 shrink-0 animate-fade-up overflow-hidden bg-ink-800 shadow-2xl shadow-black/60 ring-1 ring-white/10 sm:w-60",
            round ? "aspect-square rounded-full" : portrait ? "aspect-[2/3] rounded-2xl" : "aspect-square rounded-2xl",
          )}
        >
          <Img src={image} className="absolute inset-0 h-full w-full object-cover" fallback={<MediaPlaceholder item={item} />} />
        </div>
        <div className="min-w-0 animate-fade-up">
          <div className="mb-2 text-[12px] font-semibold tracking-[0.18em] text-accent-400 uppercase">{kicker}</div>
          <h1 className="mb-3 font-display text-4xl leading-[1.05] font-bold tracking-tight text-balance sm:text-5xl lg:text-6xl">{item.name}</h1>
          <div className="mb-6 text-[15px] text-ink-300">{meta}</div>
          <div className="flex flex-wrap justify-center gap-3 sm:justify-start">{actions}</div>
        </div>
      </div>
    </div>
  );
}

export function totalDuration(items: Item[]) {
  return formatDuration(items.reduce((a, b) => a + (b.duration ?? 0), 0));
}

// ---- Photos ----

export function PhotoGrid({ photos, onOpen }: { photos: Item[]; onOpen: (i: number) => void }) {
  return (
    <div className="grid grid-cols-3 gap-1 sm:grid-cols-4 sm:gap-1.5 md:grid-cols-5 lg:grid-cols-6 xl:grid-cols-8">
      {photos.map((p, i) => (
        <button key={p.id} onClick={() => onOpen(i)} className="group focus-ring relative aspect-square overflow-hidden rounded-lg bg-ink-800">
          <Img src={imageUrl(p.id, "primary", p.images.primary, 400)} className="h-full w-full object-cover transition duration-500 group-hover:scale-105" />
        </button>
      ))}
    </div>
  );
}

export function groupByMonth(photos: Item[]) {
  const groups: { key: string; label: string; items: Item[] }[] = [];
  for (const p of photos) {
    const d = new Date((p.takenAt || p.dateAdded) * 1000);
    const key = `${d.getFullYear()}-${d.getMonth()}`;
    let g = groups[groups.length - 1];
    if (!g || g.key !== key) {
      g = { key, label: d.toLocaleDateString(locale(), { month: "long", year: "numeric" }), items: [] };
      groups.push(g);
    }
    g.items.push(p);
  }
  return groups;
}

export function Lightbox({ photos, index, onClose }: { photos: Item[]; index: number; onClose: () => void }) {
  const [i, setI] = useState(index);
  const [zoom, setZoom] = useState(false);
  const [slideshow, setSlideshow] = useState(false);
  const touch = useRef<number | null>(null);
  const p = photos[i];
  const go = useCallback((d: number) => {
    setZoom(false);
    setI((x) => (x + d + photos.length) % photos.length);
  }, [photos.length]);
  useEffect(() => {
    const k = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
      if (e.key === "ArrowRight") go(1);
      if (e.key === "ArrowLeft") go(-1);
      if (e.key === " ") {
        e.preventDefault();
        setSlideshow((s) => !s);
      }
    };
    window.addEventListener("keydown", k);
    const prev = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      window.removeEventListener("keydown", k);
      document.body.style.overflow = prev;
    };
  }, [go, onClose]);
  useEffect(() => {
    if (!slideshow) return;
    const id = setInterval(() => go(1), 4000);
    return () => clearInterval(id);
  }, [slideshow, go]);
  // Preload neighbours
  useEffect(() => {
    for (const d of [1, -1]) {
      const n = photos[(i + d + photos.length) % photos.length];
      if (n) new Image().src = imageUrl(n.id, "primary", n.images.primary, 2560);
    }
  }, [i, photos]);
  if (!p) return null;
  const date = p.takenAt ? new Date(p.takenAt * 1000).toLocaleString(locale(), { dateStyle: "long", timeStyle: "short" }) : "";
  return createPortal(
    <div
      className="fixed inset-0 z-[150] flex animate-fade-in flex-col bg-black"
      onTouchStart={(e) => (touch.current = e.touches[0].clientX)}
      onTouchEnd={(e) => {
        if (touch.current === null) return;
        const dx = e.changedTouches[0].clientX - touch.current;
        if (Math.abs(dx) > 50) go(dx < 0 ? 1 : -1);
        touch.current = null;
      }}
    >
      <div className="absolute inset-x-0 top-0 z-10 flex items-center gap-3 bg-gradient-to-b from-black/80 to-transparent p-4">
        <div className="min-w-0 flex-1">
          <div className="truncate font-medium">{p.name}</div>
          <div className="truncate text-xs text-ink-400">
            {[date, p.width && p.height ? `${p.width}×${p.height}` : "", p.studios?.[0]].filter(Boolean).join(" · ")} · {i + 1}/{photos.length}
          </div>
        </div>
        <button onClick={() => setSlideshow((s) => !s)} className={clsx("grid h-10 w-10 place-items-center rounded-full hover:bg-white/10", slideshow && "text-accent-400")} title={t("Slideshow")}>
          {slideshow ? <Pause size={19} /> : <Timer size={19} />}
        </button>
        <a href={`/api/items/${p.id}/file?download=1`} className="grid h-10 w-10 place-items-center rounded-full hover:bg-white/10" title={t("Download")}>
          <Download size={19} />
        </a>
        <button onClick={onClose} className="grid h-10 w-10 place-items-center rounded-full hover:bg-white/10" aria-label={t("Close")}>
          <X size={22} />
        </button>
      </div>
      <div className="relative flex flex-1 items-center justify-center overflow-hidden" onClick={() => setZoom((z) => !z)}>
        <img
          key={p.id}
          src={imageUrl(p.id, "primary", p.images.primary, 2560)}
          alt={p.name}
          className={clsx("max-h-full max-w-full animate-fade-in object-contain transition-transform duration-300 select-none", zoom ? "scale-[2] cursor-zoom-out" : "cursor-zoom-in")}
          draggable={false}
        />
      </div>
      {photos.length > 1 && (
        <>
          <button onClick={() => go(-1)} className="absolute top-1/2 left-3 hidden h-12 w-12 -translate-y-1/2 place-items-center rounded-full bg-black/40 hover:bg-white/15 sm:grid" aria-label="Previous">
            <ChevronLeft size={26} />
          </button>
          <button onClick={() => go(1)} className="absolute top-1/2 right-3 hidden h-12 w-12 -translate-y-1/2 place-items-center rounded-full bg-black/40 hover:bg-white/15 sm:grid" aria-label={t("Next")}>
            <ChevronRight size={26} />
          </button>
        </>
      )}
    </div>,
    document.body,
  );
}
