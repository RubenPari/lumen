import { useQuery, useQueryClient } from "@tanstack/react-query";
import clsx from "clsx";
import { Check, ChevronDown, Clapperboard, Heart, Pencil, Play, RefreshCw, RotateCcw, User as UserIcon } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { Link, useNavigate, useParams, useSearchParams } from "react-router-dom";
import { Img, PosterCard, Row } from "../components/cards";
import { Button, EmptyState, IconButton, Input, Modal, Spinner, Toggle, useToast } from "../components/ui";
import { api, backdropImage, landscapeImage, logoImage, posterImage, type Item, type Person } from "../lib/api";
import { channelsLabel, endsAt, episodeLabel, formatDuration, progressPct, resolutionLabel } from "../lib/format";
import { useItem, useMe, useUserDataActions } from "../lib/hooks";
import { t } from "../lib/i18n";
import { MetaLine } from "./Home";

function TechBadges({ item }: { item: Item }) {
  const streams = item.media?.streams ?? [];
  const v = streams.find((s) => s.type === "video");
  const a = streams.find((s) => s.type === "audio");
  const badges = [
    resolutionLabel(v?.width, v?.height),
    v?.hdr ? "HDR" : undefined,
    v?.codec?.toUpperCase().replace("HEVC", "HEVC"),
    a ? [a.codec?.toUpperCase(), channelsLabel(a.channels)].filter(Boolean).join(" ") : undefined,
    item.subtitles?.length || streams.some((s) => s.type === "subtitle") ? "CC" : undefined,
  ].filter(Boolean);
  if (!badges.length) return null;
  return (
    <div className="flex flex-wrap gap-1.5">
      {badges.map((b) => (
        <span key={b} className="rounded-md bg-white/[0.08] px-2 py-0.5 text-[11px] font-semibold tracking-wide text-ink-200 ring-1 ring-white/10 ring-inset">
          {b}
        </span>
      ))}
    </div>
  );
}

function CastCard({ p }: { p: Person }) {
  return (
    <Link to={`/browse?person=${encodeURIComponent(p.name)}`} className="group focus-ring block rounded-2xl text-center">
      <div className="mx-auto aspect-square w-full overflow-hidden rounded-full bg-ink-800 ring-1 ring-white/10 transition duration-300 group-hover:scale-[1.04] group-hover:ring-white/30">
        <Img
          src={p.image}
          className="h-full w-full object-cover"
          fallback={
            <div className="grid h-full w-full place-items-center text-ink-500">
              <UserIcon size={32} strokeWidth={1.5} />
            </div>
          }
        />
      </div>
      <div className="mt-2.5 truncate text-[13px] font-medium text-ink-100">{p.name}</div>
      {p.role && <div className="truncate text-[12px] text-ink-400">{p.role}</div>}
    </Link>
  );
}

function EpisodeRow({ ep, highlight }: { ep: Item; highlight?: boolean }) {
  const navigate = useNavigate();
  const { setPlayed } = useUserDataActions();
  const [open, setOpen] = useState(false);
  const pct = progressPct(ep);
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (highlight) ref.current?.scrollIntoView({ block: "center", behavior: "smooth" });
  }, [highlight]);
  return (
    <div
      ref={ref}
      className={clsx(
        "group flex flex-col gap-4 rounded-2xl p-2.5 transition sm:flex-row sm:items-center sm:p-3",
        highlight ? "bg-white/[0.07] ring-1 ring-accent-400/40" : "hover:bg-white/[0.04]",
      )}
    >
      <button onClick={() => navigate(`/play/${ep.id}`)} className="focus-ring relative aspect-video w-full shrink-0 overflow-hidden rounded-xl bg-ink-800 sm:w-64">
        <Img
          src={landscapeImage(ep, 520)}
          className="h-full w-full object-cover transition duration-500 group-hover:scale-[1.03]"
          fallback={<div className="h-full w-full bg-gradient-to-br from-ink-700 to-ink-850" />}
        />
        <span className="absolute inset-0 grid place-items-center bg-black/30 opacity-0 transition group-hover:opacity-100">
          <span className="grid h-12 w-12 place-items-center rounded-full bg-white/95 text-ink-950 shadow-xl">
            <Play size={20} fill="currentColor" className="ml-0.5" />
          </span>
        </span>
        {pct > 0 && (
          <span className="absolute inset-x-2.5 bottom-2.5 h-1 overflow-hidden rounded-full bg-white/25">
            <span className="block h-full bg-accent-400" style={{ width: `${pct}%` }} />
          </span>
        )}
      </button>
      <div className="min-w-0 flex-1">
        <div className="mb-1 flex items-start justify-between gap-3">
          <div className="min-w-0">
            <div className="text-[12px] font-semibold tracking-wide text-ink-400">
              {ep.episodeNumber != null ? `${t("Episode")} ${ep.episodeNumber}${ep.episodeEnd && ep.episodeEnd !== ep.episodeNumber ? "–" + ep.episodeEnd : ""}` : episodeLabel(ep)}
              {ep.duration ? ` · ${formatDuration(ep.duration)}` : ""}
              {ep.premiereDate ? ` · ${new Date(ep.premiereDate).toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" })}` : ""}
            </div>
            <h3 className="truncate text-[16px] font-semibold text-white">{ep.name}</h3>
          </div>
          <button
            onClick={() => setPlayed(ep.id, !ep.userData.played)}
            title={ep.userData.played ? t("Mark as unwatched") : t("Mark as watched")}
            className={clsx(
              "grid h-8 w-8 shrink-0 place-items-center rounded-full transition",
              ep.userData.played ? "bg-accent-500 text-ink-950" : "text-ink-400 ring-1 ring-white/15 hover:bg-white/10 hover:text-white",
            )}
          >
            <Check size={15} strokeWidth={3} />
          </button>
        </div>
        {ep.overview && (
          <button onClick={() => setOpen((o) => !o)} className="text-left">
            <p className={clsx("text-[14px] leading-relaxed text-ink-300", !open && "line-clamp-2")}>{ep.overview}</p>
          </button>
        )}
      </div>
    </div>
  );
}

function Seasons({ series, selected, onSelect, highlightEp }: { series: Item; selected?: string; onSelect: (id: string) => void; highlightEp?: string }) {
  const { data: seasons } = useQuery({ queryKey: ["children", series.id], queryFn: () => api.get<Item[]>(`/api/items/${series.id}/children`) });
  const current = selected && seasons?.find((s) => s.id === selected) ? selected : seasons?.find((s) => s.unplayedCount)?.id ?? seasons?.[0]?.id;
  const { data: episodes, isLoading } = useQuery({
    queryKey: ["children", current],
    queryFn: () => api.get<Item[]>(`/api/items/${current}/children`),
    enabled: !!current,
  });
  const { setPlayed } = useUserDataActions();
  const season = seasons?.find((s) => s.id === current);
  if (!seasons?.length) return null;
  return (
    <section className="px-5 sm:px-10 lg:px-14">
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <div className="no-scrollbar -mx-1 flex gap-1 overflow-x-auto px-1">
          {seasons.map((s) => (
            <button
              key={s.id}
              onClick={() => onSelect(s.id)}
              className={clsx(
                "focus-ring relative h-10 shrink-0 rounded-full px-4 text-[14px] font-semibold transition",
                s.id === current ? "bg-white text-ink-950" : "text-ink-300 hover:bg-white/10 hover:text-white",
              )}
            >
              {s.name}
              {s.userData.played && <Check size={13} strokeWidth={3} className="ml-1.5 inline -translate-y-px" />}
            </button>
          ))}
        </div>
        {season && (
          <Button size="sm" variant="ghost" onClick={() => setPlayed(season.id, !season.userData.played)}>
            <Check size={15} /> {season.userData.played ? t("Mark as unwatched") : t("Mark as watched")}
          </Button>
        )}
      </div>
      {season?.overview && <p className="mb-4 max-w-3xl text-[14px] leading-relaxed text-ink-300">{season.overview}</p>}
      {isLoading ? (
        <div className="grid place-items-center py-16">
          <Spinner />
        </div>
      ) : (
        <div className="space-y-1">
          {episodes?.map((ep) => (
            <EpisodeRow key={ep.id} ep={ep} highlight={ep.id === highlightEp} />
          ))}
        </div>
      )}
    </section>
  );
}

/** Picks the episode to play for a series: in progress → next unwatched → first. */
function useSeriesNext(series?: Item) {
  const { data } = useQuery({
    queryKey: ["items", "eps", series?.id],
    queryFn: () => api.get<{ items: Item[] }>(`/api/items?seriesId=${series!.id}&type=Episode&sort=episode&limit=1000`),
    enabled: series?.type === "Series",
  });
  return useMemo(() => {
    const eps = (data?.items ?? []).slice().sort((a, b) => {
      const sa = a.seasonNumber === 0 ? 9999 : (a.seasonNumber ?? 1);
      const sb = b.seasonNumber === 0 ? 9999 : (b.seasonNumber ?? 1);
      return sa - sb || (a.episodeNumber ?? 9999) - (b.episodeNumber ?? 9999);
    });
    if (!eps.length) return undefined;
    const regular = eps.filter((e) => e.seasonNumber !== 0);
    const inProgress = eps.filter((e) => e.userData.position > 0 && !e.userData.played).sort((a, b) => (b.userData.lastPlayed ?? 0) - (a.userData.lastPlayed ?? 0))[0];
    if (inProgress) return inProgress;
    let lastPlayedIdx = -1;
    let lastTime = -1;
    regular.forEach((e, i) => {
      if (e.userData.played && (e.userData.lastPlayed ?? 0) >= lastTime) {
        lastPlayedIdx = i;
        lastTime = e.userData.lastPlayed ?? 0;
      }
    });
    return regular.slice(lastPlayedIdx + 1).find((e) => !e.userData.played) ?? regular.find((e) => !e.userData.played) ?? regular[0] ?? eps[0];
  }, [data]);
}

function EditModal({ item, open, onClose }: { item: Item; open: boolean; onClose: () => void }) {
  const qc = useQueryClient();
  const toast = useToast();
  const [form, setForm] = useState({
    name: item.name,
    originalTitle: item.originalTitle ?? "",
    sortName: item.sortName ?? "",
    year: item.year ? String(item.year) : "",
    tagline: item.tagline ?? "",
    overview: item.overview ?? "",
    officialRating: item.officialRating ?? "",
    genres: (item.genres ?? []).join(", "),
    tmdbId: item.tmdbId ?? "",
    locked: item.metadataLocked ?? false,
  });
  const [saving, setSaving] = useState(false);
  useEffect(() => {
    if (open)
      setForm((f) => ({ ...f, name: item.name, overview: item.overview ?? "", locked: item.metadataLocked ?? false }));
  }, [open, item]);
  const f = (k: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => setForm({ ...form, [k]: e.target.value });
  const save = async () => {
    setSaving(true);
    try {
      await api.patch(`/api/items/${item.id}`, {
        name: form.name,
        originalTitle: form.originalTitle,
        sortName: form.sortName,
        year: parseInt(form.year) || 0,
        tagline: form.tagline,
        overview: form.overview,
        officialRating: form.officialRating,
        genres: form.genres.split(",").map((g) => g.trim()).filter(Boolean),
        tmdbId: form.tmdbId,
        metadataLocked: form.locked,
      });
      qc.invalidateQueries();
      toast(t("Saved"), "success");
      onClose();
    } catch (e) {
      toast((e as Error).message, "error");
    } finally {
      setSaving(false);
    }
  };
  return (
    <Modal open={open} onClose={onClose} title={t("Edit metadata")} wide>
      <div className="grid gap-4 sm:grid-cols-2">
        <Input label={t("Title")} value={form.name} onChange={f("name")} />
        <Input label={t("Original title")} value={form.originalTitle} onChange={f("originalTitle")} />
        <Input label="Sort name" value={form.sortName} onChange={f("sortName")} />
        <Input label={t("Year")} value={form.year} onChange={f("year")} inputMode="numeric" />
        <Input label={t("Tagline")} value={form.tagline} onChange={f("tagline")} />
        <Input label="Rating (MPAA)" value={form.officialRating} onChange={f("officialRating")} />
        <div className="sm:col-span-2">
          <Input label={t("Genres")} value={form.genres} onChange={f("genres")} />
        </div>
        <label className="block space-y-1.5 sm:col-span-2">
          <span className="text-[13px] font-medium text-ink-300">{t("Overview")}</span>
          <textarea
            value={form.overview}
            onChange={f("overview")}
            rows={5}
            className="w-full rounded-xl bg-white/[0.06] px-4 py-3 text-[15px] text-white ring-1 ring-white/10 ring-inset focus:ring-2 focus:ring-accent-400 focus:outline-none"
          />
        </label>
        {(item.type === "Movie" || item.type === "Series") && <Input label="TMDB ID" value={form.tmdbId} onChange={f("tmdbId")} />}
        <div className="flex items-end sm:col-span-2">
          <Toggle checked={form.locked} onChange={(v) => setForm({ ...form, locked: v })} label={t("Lock metadata")} />
        </div>
      </div>
      <div className="mt-6 flex justify-end gap-2">
        <Button variant="ghost" onClick={onClose}>
          {t("Cancel")}
        </Button>
        <Button variant="primary" onClick={save} disabled={saving}>
          {saving && <Spinner className="h-4 w-4 border-ink-950/30 border-t-ink-950" />} {t("Save")}
        </Button>
      </div>
    </Modal>
  );
}

export function ItemPage() {
  const { id } = useParams();
  const [sp, setSp] = useSearchParams();
  const { data: item, isLoading, error } = useItem(id);
  const { data: me } = useMe();
  const navigate = useNavigate();
  const toast = useToast();
  const { setPlayed, setFavorite } = useUserDataActions();
  const [overviewOpen, setOverviewOpen] = useState(false);
  const [editing, setEditing] = useState(false);
  const next = useSeriesNext(item);
  const { data: similar } = useQuery({
    queryKey: ["similar", id],
    queryFn: () => api.get<Item[]>(`/api/items/${id}/similar`),
    enabled: !!item && (item.type === "Movie" || item.type === "Series"),
  });

  useEffect(() => {
    if (sp.get("autoplay") && next) navigate(`/play/${next.id}`, { replace: true });
  }, [sp, next, navigate]);

  if (isLoading)
    return (
      <div className="grid h-dvh place-items-center">
        <Spinner />
      </div>
    );
  if (error || !item) return <EmptyState title={t("Not found")} />;

  const playTarget = item.type === "Series" ? next : item;
  const resumePct = playTarget ? progressPct(playTarget) : 0;
  const remaining = playTarget?.duration ? playTarget.duration - (playTarget.userData.position || 0) : 0;
  const backdrop = backdropImage(item);
  const logo = logoImage(item);
  const cast = (item.people ?? []).filter((p) => p.type === "Actor");
  const directors = (item.people ?? []).filter((p) => p.type === "Director");
  const writers = (item.people ?? []).filter((p) => p.type === "Writer");

  const refresh = async () => {
    try {
      await api.post(`/api/items/${item.id}/refresh`);
      toast(t("Metadata refresh started"), "success");
    } catch (e) {
      toast((e as Error).message, "error");
    }
  };

  return (
    <div className="pb-20">
      {/* Backdrop */}
      <div className="absolute inset-x-0 top-0 h-[92svh] overflow-hidden">
        {backdrop ? (
          <img src={backdrop} alt="" className="h-full w-full animate-fade-in object-cover" style={{ objectPosition: "50% 20%" }} />
        ) : (
          <div className="h-full w-full bg-[radial-gradient(80%_60%_at_70%_0%,#3a2a1a,transparent)]" />
        )}
        <div className="absolute inset-0 bg-gradient-to-r from-ink-950 via-ink-950/75 to-ink-950/10" />
        <div className="absolute inset-0 bg-gradient-to-t from-ink-950 via-ink-950/40 to-black/30" />
      </div>

      <div className="relative px-5 pt-[30svh] sm:px-10 sm:pt-[34svh] lg:px-14">
        <div className="flex flex-col gap-8 md:flex-row md:items-end">
          <div className="hidden w-56 shrink-0 animate-fade-up overflow-hidden rounded-2xl shadow-2xl shadow-black/60 ring-1 ring-white/10 md:block lg:w-64">
            <Img src={posterImage(item, 520)} className="aspect-[2/3] w-full object-cover" fallback={<div className="aspect-[2/3] bg-ink-800" />} />
          </div>
          <div className="max-w-3xl min-w-0 animate-fade-up">
            {logo ? (
              <img src={logo} alt={item.name} className="mb-5 max-h-32 max-w-[min(90%,480px)] object-contain object-left drop-shadow-2xl" />
            ) : (
              <h1 className="text-shadow mb-3 font-display text-4xl leading-[1.05] font-bold tracking-tight text-balance sm:text-5xl lg:text-6xl">{item.name}</h1>
            )}
            {item.originalTitle && item.originalTitle !== item.name && <div className="mb-2 text-ink-300 italic">{item.originalTitle}</div>}
            <MetaLine item={item} className="mb-3" />
            <div className="mb-5 flex flex-wrap items-center gap-3">
              <TechBadges item={item} />
              {item.type !== "Series" && item.duration ? (
                <span className="text-[13px] text-ink-400">{t("Ends at {0}", endsAt(remaining || item.duration))}</span>
              ) : null}
            </div>

            <div className="mb-6 flex flex-wrap items-center gap-3">
              {playTarget && (
                <Button variant="primary" size="lg" onClick={() => navigate(`/play/${playTarget.id}`)} className="relative overflow-hidden">
                  <Play size={20} fill="currentColor" />
                  {resumePct > 0 ? t("Resume") : t("Play")}
                  {item.type === "Series" && playTarget && <span className="font-normal text-ink-600">{episodeLabel(playTarget)}</span>}
                  {resumePct > 0 && <span className="absolute inset-x-0 bottom-0 h-1 bg-ink-950/15"><span className="block h-full bg-accent-500" style={{ width: `${resumePct}%` }} /></span>}
                </Button>
              )}
              {resumePct > 0 && playTarget && (
                <IconButton label={t("Play from start")} onClick={() => navigate(`/play/${playTarget.id}?t=0`)} className="h-12 w-12">
                  <RotateCcw size={19} />
                </IconButton>
              )}
              {item.trailers?.[0] && (
                <IconButton label={t("Trailer")} onClick={() => navigate(`/play/${item.trailers![0]}`)} className="h-12 w-12">
                  <Clapperboard size={19} />
                </IconButton>
              )}
              <IconButton
                label={item.userData.played ? t("Mark as unwatched") : t("Mark as watched")}
                active={item.userData.played}
                onClick={() => setPlayed(item.id, !item.userData.played)}
                className="h-12 w-12"
              >
                <Check size={20} strokeWidth={2.5} />
              </IconButton>
              <IconButton
                label={item.userData.favorite ? t("Remove from favorites") : t("Add to favorites")}
                onClick={() => setFavorite(item.id, !item.userData.favorite)}
                className={clsx("h-12 w-12", item.userData.favorite && "!bg-rose-500/20 !text-rose-400 !ring-rose-400/40")}
              >
                <Heart size={19} fill={item.userData.favorite ? "currentColor" : "none"} />
              </IconButton>
              {me?.isAdmin && (
                <>
                  <IconButton label={t("Edit metadata")} onClick={() => setEditing(true)} className="h-12 w-12">
                    <Pencil size={18} />
                  </IconButton>
                  <IconButton label={t("Refresh metadata")} onClick={refresh} className="h-12 w-12">
                    <RefreshCw size={18} />
                  </IconButton>
                </>
              )}
            </div>

            {item.tagline && <p className="mb-2 font-display text-lg text-ink-100 italic">{item.tagline}</p>}
            {item.overview && (
              <button onClick={() => setOverviewOpen((o) => !o)} className="group block text-left">
                <p className={clsx("max-w-2xl text-[15px] leading-relaxed text-ink-200", !overviewOpen && "line-clamp-3")}>{item.overview}</p>
                {item.overview.length > 240 && (
                  <ChevronDown size={18} className={clsx("mt-1 text-ink-400 transition group-hover:text-white", overviewOpen && "rotate-180")} />
                )}
              </button>
            )}
            <dl className="mt-5 grid max-w-2xl gap-x-6 gap-y-1.5 text-[14px] sm:grid-cols-[auto_1fr]">
              {directors.length > 0 && (
                <>
                  <dt className="text-ink-400">{t("Director")}</dt>
                  <dd className="text-ink-200">
                    {directors.map((d, i) => (
                      <span key={d.name}>
                        {i > 0 && ", "}
                        <Link className="hover:text-white hover:underline" to={`/browse?person=${encodeURIComponent(d.name)}`}>
                          {d.name}
                        </Link>
                      </span>
                    ))}
                  </dd>
                </>
              )}
              {writers.length > 0 && (
                <>
                  <dt className="text-ink-400">{t("Writer")}</dt>
                  <dd className="text-ink-200">{writers.map((w) => w.name).join(", ")}</dd>
                </>
              )}
              {item.studios && item.studios.length > 0 && (
                <>
                  <dt className="text-ink-400">{t("Studios")}</dt>
                  <dd className="text-ink-200">
                    {item.studios.slice(0, 4).map((s, i) => (
                      <span key={s}>
                        {i > 0 && ", "}
                        <Link className="hover:text-white hover:underline" to={`/browse?studio=${encodeURIComponent(s)}`}>
                          {s}
                        </Link>
                      </span>
                    ))}
                  </dd>
                </>
              )}
              {item.genres && item.genres.length > 0 && (
                <>
                  <dt className="text-ink-400">{t("Genres")}</dt>
                  <dd className="text-ink-200">
                    {item.genres.map((g, i) => (
                      <span key={g}>
                        {i > 0 && ", "}
                        <Link className="hover:text-white hover:underline" to={`/browse?genre=${encodeURIComponent(g)}`}>
                          {g}
                        </Link>
                      </span>
                    ))}
                  </dd>
                </>
              )}
            </dl>
          </div>
        </div>
      </div>

      <div className="relative mt-14 space-y-12">
        {item.type === "Series" && (
          <Seasons
            series={item}
            selected={sp.get("season") ?? undefined}
            highlightEp={sp.get("ep") ?? undefined}
            onSelect={(s) => {
              const n = new URLSearchParams(sp);
              n.set("season", s);
              n.delete("ep");
              setSp(n, { replace: true });
            }}
          />
        )}
        {cast.length > 0 && (
          <Row title={t("Cast")}>
            {cast.slice(0, 24).map((p) => (
              <CastCard key={p.name + p.role} p={p} />
            ))}
          </Row>
        )}
        {similar && similar.length > 0 && (
          <Row title={t("More like this")}>
            {similar.map((s) => (
              <PosterCard key={s.id} item={s} />
            ))}
          </Row>
        )}
      </div>
      {me?.isAdmin && <EditModal item={item} open={editing} onClose={() => setEditing(false)} />}
    </div>
  );
}

