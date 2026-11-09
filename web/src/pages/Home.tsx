import { useQuery } from "@tanstack/react-query";
import clsx from "clsx";
import { FolderPlus, Info, Play, Star } from "lucide-react";
import { useEffect, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { LandscapeCard, PosterCard, Row, SkeletonCard } from "../components/cards";
import { MediaCard } from "../components/media";
import { Button, EmptyState } from "../components/ui";
import { api, backdropImage, logoImage, type HomeRow, type Item } from "../lib/api";
import { formatDuration, progressPct, yearRange } from "../lib/format";
import { useLibraries, useMe } from "../lib/hooks";
import { t } from "../lib/i18n";

interface HomeData {
  featured: Item[];
  rows: HomeRow[];
}

export function MetaLine({ item, className }: { item: Item; className?: string }) {
  const parts: React.ReactNode[] = [];
  if (item.communityRating)
    parts.push(
      <span key="r" className="inline-flex items-center gap-1 text-accent-300">
        <Star size={14} fill="currentColor" /> {item.communityRating.toFixed(1)}
      </span>,
    );
  const yr = yearRange(item);
  if (yr) parts.push(<span key="y">{yr}</span>);
  if (item.officialRating)
    parts.push(
      <span key="o" className="rounded-[5px] px-1.5 py-px text-[12px] font-semibold ring-1 ring-current/40 ring-inset">
        {item.officialRating}
      </span>,
    );
  if (item.type === "Series" && item.childCount) parts.push(<span key="s">{item.childCount === 1 ? t("1 season") : t("{0} seasons", item.childCount)}</span>);
  else if (item.duration) parts.push(<span key="d">{formatDuration(item.duration)}</span>);
  if (item.genres?.length) parts.push(<span key="g">{item.genres.slice(0, 3).join(" · ")}</span>);
  return (
    <div className={clsx("flex flex-wrap items-center gap-x-3 gap-y-1.5 text-[14px] font-medium text-ink-200", className)}>
      {parts.map((p, i) => (
        <span key={i} className="inline-flex items-center gap-3">
          {i > 0 && <span className="h-1 w-1 rounded-full bg-ink-500" />}
          {p}
        </span>
      ))}
    </div>
  );
}

function Hero({ items }: { items: Item[] }) {
  const [idx, setIdx] = useState(0);
  const [paused, setPaused] = useState(false);
  const navigate = useNavigate();
  useEffect(() => {
    if (items.length < 2 || paused) return;
    const id = setInterval(() => setIdx((i) => (i + 1) % items.length), 9000);
    return () => clearInterval(id);
  }, [items.length, paused]);
  const item = items[idx];
  if (!item) return null;
  const logo = logoImage(item);
  const resumable = progressPct(item) > 0;
  return (
    <section
      className="relative h-[78svh] min-h-[520px] w-full overflow-hidden sm:h-[88svh]"
      onMouseEnter={() => setPaused(true)}
      onMouseLeave={() => setPaused(false)}
    >
      {items.map((it, i) => (
        <div key={it.id} className={clsx("absolute inset-0 transition-opacity duration-[1400ms]", i === idx ? "opacity-100" : "opacity-0")}>
          {Math.abs(i - idx) <= 1 || (idx === items.length - 1 && i === 0) ? (
            <img
              src={backdropImage(it)}
              alt=""
              className={clsx("h-full w-full object-cover", i === idx && "animate-ken-burns")}
              style={{ objectPosition: "50% 20%" }}
            />
          ) : null}
        </div>
      ))}
      <div className="absolute inset-0 bg-gradient-to-r from-ink-950/95 via-ink-950/50 to-transparent" />
      <div className="absolute inset-0 bg-gradient-to-t from-ink-950 via-ink-950/20 to-black/30" />

      <div key={item.id} className="absolute inset-x-0 bottom-[14%] px-5 sm:bottom-[18%] sm:px-10 lg:px-14">
        <div className="max-w-2xl animate-fade-up">
          {logo ? (
            <img src={logo} alt={item.name} className="mb-6 max-h-28 max-w-[min(80%,460px)] object-contain object-left drop-shadow-2xl sm:max-h-36" />
          ) : (
            <h1 className="text-shadow mb-4 font-display text-4xl leading-[1.05] font-bold tracking-tight text-balance sm:text-6xl lg:text-7xl">{item.name}</h1>
          )}
          <MetaLine item={item} className="text-shadow mb-4" />
          {item.overview && <p className="text-shadow mb-7 line-clamp-3 max-w-xl text-[15px] leading-relaxed text-ink-200 sm:text-base">{item.overview}</p>}
          <div className="flex flex-wrap gap-3">
            <Button variant="primary" size="lg" onClick={() => navigate(item.type === "Series" ? `/item/${item.id}?autoplay=1` : `/play/${item.id}`)}>
              <Play size={20} fill="currentColor" /> {resumable ? t("Resume") : t("Play")}
            </Button>
            <Button variant="glass" size="lg" onClick={() => navigate(`/item/${item.id}`)}>
              <Info size={20} /> {t("More info")}
            </Button>
          </div>
        </div>
      </div>
      {items.length > 1 && (
        <div className="absolute right-5 bottom-[14%] flex gap-2 sm:right-10 sm:bottom-[18%] lg:right-14">
          {items.map((it, i) => (
            <button
              key={it.id}
              aria-label={it.name}
              onClick={() => setIdx(i)}
              className={clsx("h-1.5 rounded-full transition-all duration-500", i === idx ? "w-8 bg-white" : "w-3 bg-white/35 hover:bg-white/60")}
            />
          ))}
        </div>
      )}
    </section>
  );
}

export function Home() {
  const { data, isLoading } = useQuery({ queryKey: ["home"], queryFn: () => api.get<HomeData>("/api/home") });
  const { data: libs } = useLibraries();
  const { data: me } = useMe();

  if (isLoading)
    return (
      <div className="pt-24">
        <div className="skeleton mx-5 mb-10 h-[56svh] rounded-3xl sm:mx-10 lg:mx-14" />
        <div className="grid grid-cols-2 gap-4 px-5 sm:grid-cols-4 sm:px-10 lg:grid-cols-6 lg:px-14">
          {Array.from({ length: 6 }).map((_, i) => (
            <SkeletonCard key={i} />
          ))}
        </div>
      </div>
    );

  if (libs && libs.length === 0)
    return (
      <div className="px-5 pt-32">
        <EmptyState icon={<FolderPlus size={28} />} title={t("No libraries yet")}>
          <p className="mb-6">{t("Add a library to start building your collection.")}</p>
          {me?.isAdmin && (
            <Link to="/admin/libraries">
              <Button variant="primary">{t("Add library")}</Button>
            </Link>
          )}
        </EmptyState>
      </div>
    );

  const libName = (id?: string) => libs?.find((l) => l.id === id)?.name ?? "";
  const titleOf = (r: HomeRow) =>
    ({ resume: t("Continue watching"), nextup: t("Next up"), favorites: t("Your favorites"), latest: t("Recently added in {0}", libName(r.libraryId)) })[r.kind];

  return (
    <div>
      {data?.featured.length ? <Hero items={data.featured} /> : <div className="h-24" />}
      <div className={clsx("relative z-10 space-y-8 pb-16 sm:space-y-10", data?.featured.length && "-mt-[10svh]")}>
        {data?.rows.map((r) => {
          const landscape = (r.kind === "resume" || r.kind === "nextup") && r.items.some((i) => i.type === "Movie" || i.type === "Episode");
          return (
            <Row
              key={r.id}
              title={titleOf(r)}
              landscape={landscape}
              action={
                r.kind === "latest" && r.libraryId ? (
                  <Link to={`/library/${r.libraryId}?sort=dateAdded&order=desc`} className="text-[13px] font-medium text-ink-400 transition hover:text-white">
                    {t("See all")}
                  </Link>
                ) : undefined
              }
            >
              {r.items.map((it) =>
                landscape && (it.type === "Movie" || it.type === "Episode") ? (
                  <LandscapeCard key={it.id} item={it} />
                ) : isMedia(it) ? (
                  <MediaCard key={it.id} item={it} />
                ) : (
                  <PosterCard key={it.id} item={it} />
                ),
              )}
            </Row>
          );
        })}
      </div>
    </div>
  );
}

const isMedia = (it: Item) => ["MusicAlbum", "MusicArtist", "AudioBook", "Book", "PhotoAlbum"].includes(it.type);
