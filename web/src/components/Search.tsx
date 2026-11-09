import { useQuery } from "@tanstack/react-query";
import clsx from "clsx";
import { CornerDownLeft, Search as SearchIcon, User as UserIcon } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { useNavigate } from "react-router-dom";
import { api, landscapeImage, posterImage, type Item } from "../lib/api";
import { episodeLabel } from "../lib/format";
import { useDebounced } from "../lib/hooks";
import { t } from "../lib/i18n";
import { Img, itemLink } from "./cards";
import { Spinner } from "./ui";

interface SearchResult {
  items: Item[];
  people: { name: string; image?: string; count: number }[];
}

type Entry = { key: string; href: string; title: string; sub: string; img?: string; round?: boolean; landscape?: boolean };

export function SearchPalette({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [q, setQ] = useState("");
  const dq = useDebounced(q.trim(), 180);
  const navigate = useNavigate();
  const inputRef = useRef<HTMLInputElement>(null);
  const [active, setActive] = useState(0);
  const { data, isFetching } = useQuery({
    queryKey: ["search", dq],
    queryFn: () => api.get<SearchResult>(`/api/search?q=${encodeURIComponent(dq)}`),
    enabled: open && dq.length > 0,
    placeholderData: (prev) => prev,
  });

  useEffect(() => {
    if (open) {
      setQ("");
      setActive(0);
      setTimeout(() => inputRef.current?.focus(), 10);
    }
  }, [open]);

  const groups = useMemo(() => {
    if (!data || !dq) return [];
    const typeName: Record<string, string> = {
      Movie: t("Movies"),
      Series: t("Shows"),
      Episode: t("Episodes"),
      MusicArtist: t("Artists"),
      MusicAlbum: t("Albums"),
      Audio: t("Songs"),
      Book: t("Books"),
      AudioBook: t("Audiobooks"),
    };
    const g: { label: string; entries: Entry[] }[] = [];
    for (const type of ["Movie", "Series", "MusicArtist", "MusicAlbum", "Book", "AudioBook", "Episode", "Audio"]) {
      const items = data.items.filter((i) => i.type === type).slice(0, type === "Episode" ? 6 : 8);
      if (items.length)
        g.push({
          label: typeName[type],
          entries: items.map((i) => ({
            key: i.id,
            href: i.type === "Audio" && i.albumId ? `/item/${i.albumId}` : itemLink(i),
            title: i.name,
            sub:
              i.type === "Episode"
                ? `${i.seriesName} · ${episodeLabel(i)}`
                : i.type === "Audio" || i.type === "MusicAlbum"
                  ? [i.albumArtist ?? i.artists?.join(", "), i.year].filter(Boolean).join(" · ")
                  : i.type === "Book" || i.type === "AudioBook"
                    ? [i.artists?.join(", "), i.year].filter(Boolean).join(" · ")
                    : [i.year, i.genres?.slice(0, 2).join(", ")].filter(Boolean).join(" · "),
            img: i.type === "Episode" ? landscapeImage(i, 200) : i.type === "Audio" && i.albumId ? `/api/items/${i.albumId}/images/primary?w=120` : posterImage(i, 120),
            round: i.type === "MusicArtist",
            landscape: i.type === "Episode",
          })),
        });
    }
    if (data.people.length)
      g.push({
        label: t("People"),
        entries: data.people.map((p) => ({
          key: "p:" + p.name,
          href: `/browse?person=${encodeURIComponent(p.name)}`,
          title: p.name,
          sub: t("{0} items", p.count),
          img: p.image,
          round: true,
        })),
      });
    return g;
  }, [data, dq]);

  const flat = groups.flatMap((g) => g.entries);
  useEffect(() => {
    setActive(0);
  }, [dq]);

  if (!open) return null;

  const go = (e?: Entry) => {
    if (!e) return;
    onClose();
    navigate(e.href);
  };

  return createPortal(
    <div className="fixed inset-0 z-[120] flex justify-center px-3 pt-[8vh] sm:pt-[12vh]">
      <div className="absolute inset-0 animate-fade-in bg-black/70 backdrop-blur-md" onClick={onClose} />
      <div className="relative flex max-h-[76vh] w-full max-w-2xl animate-scale-in flex-col overflow-hidden rounded-3xl bg-ink-850/95 shadow-2xl ring-1 ring-white/10 backdrop-blur-xl">
        <div className="flex items-center gap-3 border-b border-white/[0.07] px-5">
          <SearchIcon size={20} className="text-ink-400" />
          <input
            ref={inputRef}
            value={q}
            onChange={(e) => setQ(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Escape") onClose();
              if (e.key === "ArrowDown") {
                e.preventDefault();
                setActive((a) => Math.min(a + 1, flat.length - 1));
              }
              if (e.key === "ArrowUp") {
                e.preventDefault();
                setActive((a) => Math.max(a - 1, 0));
              }
              if (e.key === "Enter") go(flat[active]);
            }}
            placeholder={t("Search movies, shows, people…")}
            className="h-16 flex-1 bg-transparent text-lg text-white placeholder:text-ink-400 focus:outline-none"
          />
          {isFetching && <Spinner className="h-4 w-4" />}
          <kbd className="hidden rounded-md bg-white/10 px-2 py-1 text-[11px] text-ink-300 sm:block">ESC</kbd>
        </div>
        <div className="overflow-y-auto p-2">
          {!dq && <div className="px-4 py-12 text-center text-ink-400">{t("Type to search your library")}</div>}
          {dq && data && flat.length === 0 && !isFetching && <div className="px-4 py-12 text-center text-ink-400">{t("No results")}</div>}
          {groups.map((g) => (
            <div key={g.label} className="mb-1">
              <div className="px-3 pt-3 pb-1.5 text-[11px] font-semibold tracking-wider text-ink-400 uppercase">{g.label}</div>
              {g.entries.map((e) => {
                const idx = flat.indexOf(e);
                return (
                  <button
                    key={e.key}
                    onMouseEnter={() => setActive(idx)}
                    onClick={() => go(e)}
                    className={clsx("flex w-full items-center gap-3.5 rounded-2xl px-3 py-2 text-left transition", idx === active ? "bg-white/10" : "hover:bg-white/5")}
                  >
                    <span
                      className={clsx(
                        "relative shrink-0 overflow-hidden bg-ink-700",
                        e.round ? "h-11 w-11 rounded-full" : e.landscape ? "h-11 w-[78px] rounded-lg" : "h-14 w-10 rounded-lg",
                      )}
                    >
                      <Img
                        src={e.img}
                        className="h-full w-full object-cover"
                        fallback={
                          <span className="grid h-full w-full place-items-center text-ink-400">
                            {e.round ? <UserIcon size={18} /> : <SearchIcon size={14} />}
                          </span>
                        }
                      />
                    </span>
                    <span className="min-w-0 flex-1">
                      <span className="block truncate font-medium text-white">{e.title}</span>
                      {e.sub && <span className="block truncate text-[13px] text-ink-400">{e.sub}</span>}
                    </span>
                    {idx === active && <CornerDownLeft size={16} className="text-ink-400" />}
                  </button>
                );
              })}
            </div>
          ))}
        </div>
      </div>
    </div>,
    document.body,
  );
}
