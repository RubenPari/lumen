import { useQuery } from "@tanstack/react-query";
import clsx from "clsx";
import { BookOpen, Check, Download, Headphones, Heart, Play, RotateCcw } from "lucide-react";
import { useSearchParams, useNavigate } from "react-router-dom";
import { Row } from "../components/cards";
import { MediaCard, MediaHeader, TrackList, totalDuration } from "../components/media";
import { Button, Chip, IconButton, Select, Spinner } from "../components/ui";
import { api, posterImage, type Item, type Library } from "../lib/api";
import { useAudio } from "../lib/audio";
import { formatBytes, formatClock } from "../lib/format";
import { useUserDataActions } from "../lib/hooks";
import { t } from "../lib/i18n";
import { ItemGrid } from "./Library";

export function BookLibrary({ lib }: { lib: Library }) {
  const [sp, setSp] = useSearchParams();
  const tab = sp.get("tab") ?? "all";
  const sort = sp.get("sort") ?? "dateAdded";
  const set = (k: string, v: string) => {
    const n = new URLSearchParams(sp);
    n.set(k, v);
    setSp(n, { replace: true });
  };
  const type = { all: "Book,AudioBook", books: "Book", audiobooks: "AudioBook" }[tab] ?? "Book,AudioBook";
  const params = new URLSearchParams({ libraryId: lib.id, type, sort, order: sort === "name" || sort === "series" ? "asc" : "desc" });
  if (sp.get("filter")) params.set("filter", sp.get("filter")!);
  const header = (
    <div className="mb-6 space-y-5">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <h1 className="font-display text-3xl font-bold tracking-tight sm:text-4xl">{lib.name}</h1>
        <Select
          value={sort}
          onChange={(v) => set("sort", v)}
          options={[
            { value: "dateAdded", label: t("Date added") },
            { value: "name", label: t("Title") },
            { value: "series", label: t("Series") },
            { value: "year", label: t("Release year") },
            { value: "lastPlayed", label: t("Last read") },
          ]}
        />
      </div>
      <div className="no-scrollbar flex gap-2 overflow-x-auto">
        {[
          ["all", t("All")],
          ["books", t("Books")],
          ["audiobooks", t("Audiobooks")],
        ].map(([v, l]) => (
          <Chip key={v} active={tab === v} onClick={() => set("tab", v)}>
            {l}
          </Chip>
        ))}
        <span className="mx-1 w-px shrink-0 bg-white/10" />
        {[
          ["", t("All")],
          ["resumable", t("In progress")],
          ["unplayed", t("Unread")],
          ["favorites", t("Favorites")],
        ].map(([v, l]) => (
          <Chip key={"f" + v} active={(sp.get("filter") ?? "") === v} onClick={() => set("filter", v)}>
            {l}
          </Chip>
        ))}
      </div>
    </div>
  );
  return (
    <div className="px-5 pt-24 pb-32 sm:px-10 lg:px-14">
      <ItemGrid key={tab} params={params} header={header} renderItem={(it) => <MediaCard key={it.id} item={it} />} />
    </div>
  );
}

const formatLabel = (path?: string, container?: string) => (container || path?.split(".").pop() || "").toUpperCase();

export function BookView({ book }: { book: Item }) {
  const navigate = useNavigate();
  const { setPlayed, setFavorite } = useUserDataActions();
  const { data: series } = useQuery({
    queryKey: ["items", "book-series", book.albumArtist],
    queryFn: () => api.get<{ items: Item[] }>(`/api/items?type=Book&libraryId=${book.libraryId}&sort=series&limit=100&bookSeries=${encodeURIComponent(book.albumArtist!)}`),
    enabled: !!book.albumArtist,
    select: (r) => r.items.filter((b) => b.id !== book.id),
  });
  const pct = book.userData.played ? 100 : Math.round(book.userData.position || 0);
  const readable = ["epub", "pdf", "cbz", "cbr"].includes((book.container ?? "").toLowerCase());
  return (
    <div className="pb-32">
      <MediaHeader
        item={book}
        image={posterImage(book, 600)}
        portrait
        kicker={book.albumArtist ? `${book.albumArtist}${book.trackNumber ? ` · #${book.trackNumber}` : ""}` : t("Book")}
        meta={
          <>
            {book.artists?.length ? <span className="font-semibold text-white">{book.artists.join(", ")}</span> : null}
            {[book.year, formatLabel(book.path, book.container), book.size ? formatBytes(book.size) : ""]
              .filter(Boolean)
              .map((x) => ` · ${x}`)
              .join("")}
            {pct > 0 && (
              <div className="mx-auto mt-3 flex max-w-xs items-center gap-3 sm:mx-0">
                <div className="h-1.5 flex-1 overflow-hidden rounded-full bg-white/15">
                  <div className="h-full bg-accent-400" style={{ width: `${pct}%` }} />
                </div>
                <span className="text-xs tabular-nums">{pct}%</span>
              </div>
            )}
          </>
        }
        actions={
          <>
            {readable && (
              <Button variant="primary" size="lg" onClick={() => navigate(`/read/${book.id}`)}>
                <BookOpen size={19} /> {pct > 0 && pct < 100 ? t("Continue reading") : t("Read")}
              </Button>
            )}
            <a href={`/api/items/${book.id}/file?download=1`}>
              <Button variant="glass" size="lg">
                <Download size={19} /> {t("Download")}
              </Button>
            </a>
            <IconButton label={book.userData.played ? t("Mark as unread") : t("Mark as read")} active={book.userData.played} className="h-12 w-12" onClick={() => setPlayed(book.id, !book.userData.played)}>
              <Check size={19} />
            </IconButton>
            <IconButton
              label={t("Favorite")}
              className={clsx("h-12 w-12", book.userData.favorite && "!bg-rose-500/20 !text-rose-400")}
              onClick={() => setFavorite(book.id, !book.userData.favorite)}
            >
              <Heart size={19} fill={book.userData.favorite ? "currentColor" : "none"} />
            </IconButton>
          </>
        }
      />
      {book.overview && <p className="mx-auto mb-8 max-w-3xl px-5 leading-relaxed whitespace-pre-line text-ink-200">{book.overview}</p>}
      {book.genres?.length ? (
        <div className="mx-auto mb-10 flex max-w-3xl flex-wrap gap-2 px-5">
          {book.genres.map((g) => (
            <span key={g} className="rounded-full bg-white/[0.07] px-3 py-1 text-xs text-ink-300 ring-1 ring-white/10">
              {g}
            </span>
          ))}
        </div>
      ) : null}
      {series && series.length > 0 && (
        <Row title={t("More in {0}", book.albumArtist ?? "")}>
          {series.map((b) => (
            <MediaCard key={b.id} item={b} />
          ))}
        </Row>
      )}
    </div>
  );
}

export function AudioBookView({ book }: { book: Item }) {
  const { data: chapters } = useQuery({ queryKey: ["children", book.id], queryFn: () => api.get<Item[]>(`/api/items/${book.id}/children`) });
  const audio = useAudio();
  const { setPlayed, setFavorite } = useUserDataActions();
  const pos = book.userData.played ? 0 : book.userData.position || 0;
  const pct = book.duration ? Math.round((pos / book.duration) * 100) : 0;
  return (
    <div className="pb-32">
      <MediaHeader
        item={book}
        image={posterImage(book, 600)}
        kicker={t("Audiobook")}
        meta={
          <>
            {book.artists?.length ? <span className="font-semibold text-white">{book.artists.join(", ")}</span> : null}
            {[book.year, t("{0} chapters", chapters?.length ?? 0), totalDuration(chapters ?? [])]
              .filter(Boolean)
              .map((x) => ` · ${x}`)
              .join("")}
            {pct > 0 && (
              <div className="mt-2 text-sm text-accent-300">
                {formatClock(pos)} / {formatClock(book.duration ?? 0)} ({pct}%)
              </div>
            )}
          </>
        }
        actions={
          <>
            <Button variant="primary" size="lg" onClick={() => audio.playItem(book)}>
              {pct > 0 ? <Headphones size={19} /> : <Play size={19} fill="currentColor" />} {pct > 0 ? t("Resume") : t("Listen")}
            </Button>
            {pct > 0 && (
              <IconButton
                label={t("Play from start")}
                className="h-12 w-12"
                onClick={() => audio.playItem({ ...book, userData: { ...book.userData, position: 0 } })}
              >
                <RotateCcw size={18} />
              </IconButton>
            )}
            <IconButton label={book.userData.played ? t("Mark as unwatched") : t("Mark as watched")} active={book.userData.played} className="h-12 w-12" onClick={() => setPlayed(book.id, !book.userData.played)}>
              <Check size={19} />
            </IconButton>
            <IconButton
              label={t("Favorite")}
              className={clsx("h-12 w-12", book.userData.favorite && "!bg-rose-500/20 !text-rose-400")}
              onClick={() => setFavorite(book.id, !book.userData.favorite)}
            >
              <Heart size={19} fill={book.userData.favorite ? "currentColor" : "none"} />
            </IconButton>
          </>
        }
      />
      {book.overview && <p className="mx-auto mb-8 max-w-3xl px-5 text-ink-200">{book.overview}</p>}
      <div className="mx-auto max-w-5xl px-3 sm:px-8">{chapters ? <TrackList tracks={chapters} book={book} /> : <Spinner />}</div>
    </div>
  );
}
