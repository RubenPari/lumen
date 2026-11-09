import clsx from "clsx";
import { Check, ChevronLeft, ChevronRight, Film, Heart, Play, Tv } from "lucide-react";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { Link, useNavigate } from "react-router-dom";
import { landscapeImage, posterImage, type Item } from "../lib/api";
import { episodeLabel, formatDuration, progressPct } from "../lib/format";
import { t } from "../lib/i18n";

export function Img({ src, alt = "", className, fallback }: { src?: string; alt?: string; className?: string; fallback?: ReactNode }) {
  const [state, setState] = useState<"loading" | "ok" | "error">(src ? "loading" : "error");
  useEffect(() => {
    setState(src ? "loading" : "error");
  }, [src]);
  if (!src || state === "error") return <>{fallback}</>;
  return (
    <img
      src={src}
      alt={alt}
      loading="lazy"
      decoding="async"
      draggable={false}
      onLoad={() => setState("ok")}
      onError={() => setState("error")}
      className={clsx("transition-opacity duration-500", state === "ok" ? "opacity-100" : "opacity-0", className)}
    />
  );
}

function Placeholder({ item, landscape }: { item: Item; landscape?: boolean }) {
  const hue = [...item.name].reduce((a, c) => a + c.charCodeAt(0), 0) % 360;
  const Icon = item.type === "Movie" ? Film : Tv;
  return (
    <div
      className="absolute inset-0 flex flex-col items-center justify-center gap-3 p-4 text-center"
      style={{ background: `radial-gradient(120% 90% at 30% 10%, hsl(${hue} 35% 26%), hsl(${(hue + 50) % 360} 30% 10%))` }}
    >
      <Icon className="text-white/30" size={landscape ? 28 : 32} strokeWidth={1.5} />
      <span className={clsx("line-clamp-3 font-display font-semibold text-white/85", landscape ? "text-base" : "text-[15px] leading-snug")}>
        {item.seriesName && landscape ? item.seriesName : item.name}
      </span>
    </div>
  );
}

function ProgressBar({ pct }: { pct: number }) {
  if (pct <= 0) return null;
  return (
    <div className="absolute inset-x-3 bottom-3 h-1 overflow-hidden rounded-full bg-white/25 backdrop-blur">
      <div className="h-full rounded-full bg-accent-400" style={{ width: `${Math.max(pct, 3)}%` }} />
    </div>
  );
}

function Badges({ item }: { item: Item }) {
  return (
    <div className="absolute top-2.5 right-2.5 flex gap-1.5">
      {item.userData.favorite && (
        <span className="grid h-6 w-6 place-items-center rounded-full bg-black/55 text-rose-400 backdrop-blur">
          <Heart size={12} fill="currentColor" />
        </span>
      )}
      {item.userData.played ? (
        <span className="grid h-6 w-6 place-items-center rounded-full bg-accent-500 text-ink-950 shadow">
          <Check size={14} strokeWidth={3} />
        </span>
      ) : item.type === "Series" && item.unplayedCount ? (
        <span className="grid h-6 min-w-6 place-items-center rounded-full bg-accent-500 px-1.5 text-[11px] font-bold text-ink-950 shadow">
          {item.unplayedCount}
        </span>
      ) : null}
    </div>
  );
}

export function itemLink(item: Item) {
  if (item.type === "Episode" && item.seriesId) return `/item/${item.seriesId}?season=${item.seasonId}&ep=${item.id}`;
  if (item.type === "Season" && item.seriesId) return `/item/${item.seriesId}?season=${item.id}`;
  return `/item/${item.id}`;
}

export function PosterCard({ item, className }: { item: Item; className?: string }) {
  const sub = item.type === "Series" ? item.year && String(item.year) : item.year ? String(item.year) : "";
  return (
    <Link to={itemLink(item)} className={clsx("group focus-ring block rounded-2xl", className)} draggable={false}>
      <div className="relative aspect-[2/3] overflow-hidden rounded-2xl bg-ink-800 shadow-lg shadow-black/40 ring-1 ring-white/[0.07] transition duration-300 ease-out group-hover:-translate-y-1 group-hover:shadow-2xl group-hover:shadow-black/60 group-hover:ring-white/25">
        <Img src={posterImage(item)} alt={item.name} className="absolute inset-0 h-full w-full object-cover" fallback={<Placeholder item={item} />} />
        <div className="absolute inset-0 bg-gradient-to-t from-black/50 via-transparent to-transparent opacity-0 transition duration-300 group-hover:opacity-100" />
        <Badges item={item} />
        <ProgressBar pct={progressPct(item)} />
      </div>
      <div className="mt-2.5 px-0.5">
        <div className="truncate text-[14px] font-medium text-ink-100 group-hover:text-white">{item.name}</div>
        {sub && <div className="truncate text-[13px] text-ink-400">{sub}</div>}
      </div>
    </Link>
  );
}

export function LandscapeCard({ item, className, showSeries = true }: { item: Item; className?: string; showSeries?: boolean }) {
  const navigate = useNavigate();
  const pct = progressPct(item);
  const remaining = item.duration ? item.duration - (item.userData.position || 0) : 0;
  const title = item.type === "Episode" && showSeries ? item.seriesName || item.name : item.name;
  const sub =
    item.type === "Episode"
      ? `${episodeLabel(item)}${showSeries ? " · " + item.name : ""}`
      : pct > 0
        ? t("{0} left", formatDuration(remaining))
        : item.year
          ? String(item.year)
          : "";
  return (
    <Link to={itemLink(item)} className={clsx("group focus-ring block rounded-2xl", className)} draggable={false}>
      <div className="relative aspect-video overflow-hidden rounded-2xl bg-ink-800 shadow-lg shadow-black/40 ring-1 ring-white/[0.07] transition duration-300 ease-out group-hover:-translate-y-1 group-hover:shadow-2xl group-hover:ring-white/25">
        <Img src={landscapeImage(item)} alt={item.name} className="absolute inset-0 h-full w-full object-cover" fallback={<Placeholder item={item} landscape />} />
        <div className="absolute inset-0 bg-gradient-to-t from-black/70 via-black/0 to-transparent" />
        <button
          onClick={(e) => {
            e.preventDefault();
            navigate(`/play/${item.id}`);
          }}
          aria-label={t("Play")}
          className="absolute inset-0 m-auto grid h-14 w-14 scale-75 place-items-center rounded-full bg-white/95 text-ink-950 opacity-0 shadow-2xl transition duration-300 group-hover:scale-100 group-hover:opacity-100"
        >
          <Play size={22} fill="currentColor" className="ml-0.5" />
        </button>
        <Badges item={item} />
        <ProgressBar pct={pct} />
      </div>
      <div className="mt-2.5 px-0.5">
        <div className="truncate text-[14px] font-medium text-ink-100 group-hover:text-white">{title}</div>
        {sub && <div className="truncate text-[13px] text-ink-400">{sub}</div>}
      </div>
    </Link>
  );
}

export function SkeletonCard({ landscape }: { landscape?: boolean }) {
  return (
    <div>
      <div className={clsx("skeleton rounded-2xl", landscape ? "aspect-video" : "aspect-[2/3]")} />
      <div className="skeleton mt-3 h-3.5 w-3/4 rounded-full" />
    </div>
  );
}

/** Horizontally scrolling row with arrow buttons. */
export function Row({
  title,
  action,
  children,
  landscape,
}: {
  title: ReactNode;
  action?: ReactNode;
  children: ReactNode;
  landscape?: boolean;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const [edges, setEdges] = useState({ start: true, end: false });
  const update = () => {
    const el = ref.current;
    if (!el) return;
    setEdges({ start: el.scrollLeft < 8, end: el.scrollLeft + el.clientWidth >= el.scrollWidth - 8 });
  };
  useEffect(() => {
    update();
    const el = ref.current;
    if (!el) return;
    const ro = new ResizeObserver(update);
    ro.observe(el);
    return () => ro.disconnect();
  }, [children]);
  const scroll = (dir: number) => {
    const el = ref.current;
    if (el) el.scrollBy({ left: dir * el.clientWidth * 0.85, behavior: "smooth" });
  };
  return (
    <section className="group/row relative animate-fade-up">
      <div className="mb-3 flex items-end justify-between gap-4 px-5 sm:px-10 lg:px-14">
        <h2 className="font-display text-lg font-semibold tracking-tight text-white sm:text-xl">{title}</h2>
        {action}
      </div>
      <div className="relative">
        <div
          ref={ref}
          onScroll={update}
          className={clsx(
            "no-scrollbar grid snap-x snap-mandatory scroll-px-5 grid-flow-col gap-3 overflow-x-auto overscroll-x-contain px-5 pt-1 pb-4 sm:scroll-px-10 sm:gap-4 sm:px-10 lg:scroll-px-14 lg:px-14",
            landscape
              ? "auto-cols-[78%] sm:auto-cols-[44%] md:auto-cols-[32%] lg:auto-cols-[24%] 2xl:auto-cols-[19%]"
              : "auto-cols-[40%] sm:auto-cols-[26%] md:auto-cols-[20%] lg:auto-cols-[15.5%] xl:auto-cols-[13%] 2xl:auto-cols-[11%]",
          )}
        >
          {Array.isArray(children)
            ? children.map((c, i) => (
                <div key={i} className="snap-start">
                  {c}
                </div>
              ))
            : children}
        </div>
        {!edges.start && (
          <button
            onClick={() => scroll(-1)}
            aria-label="Previous"
            className="absolute top-0 bottom-12 left-0 hidden w-14 items-center justify-center bg-gradient-to-r from-ink-950 to-transparent text-white opacity-0 transition group-hover/row:opacity-100 sm:flex"
          >
            <ChevronLeft size={30} />
          </button>
        )}
        {!edges.end && (
          <button
            onClick={() => scroll(1)}
            aria-label="Next"
            className="absolute top-0 right-0 bottom-12 hidden w-14 items-center justify-center bg-gradient-to-l from-ink-950 to-transparent text-white opacity-0 transition group-hover/row:opacity-100 sm:flex"
          >
            <ChevronRight size={30} />
          </button>
        )}
      </div>
    </section>
  );
}

export function Grid({ children }: { children: ReactNode }) {
  return (
    <div className="grid grid-cols-2 gap-x-3 gap-y-6 sm:grid-cols-3 sm:gap-x-4 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 2xl:grid-cols-8">{children}</div>
  );
}
