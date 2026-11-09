import { useQuery, useQueryClient } from "@tanstack/react-query";
import clsx from "clsx";
import { Heart, Radio, Star, Tv } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { Img } from "../components/cards";
import { Chip, EmptyState, Spinner } from "../components/ui";
import { api } from "../lib/api";
import { useMe } from "../lib/hooks";
import { locale, t } from "../lib/i18n";

export interface LiveProgram {
  id: string;
  channelId?: string;
  start: number;
  stop: number;
  title: string;
  subTitle?: string;
  description?: string;
  category?: string;
  icon?: string;
  episode?: string;
}

export interface LiveChannel {
  id: string;
  number: string;
  name: string;
  logo?: string;
  group?: string;
  favorite: boolean;
  now?: LiveProgram;
  next?: LiveProgram;
  programs?: LiveProgram[];
}

export const hhmm = (unix: number) => new Date(unix * 1000).toLocaleTimeString(locale(), { hour: "2-digit", minute: "2-digit" });

export function useNow(intervalMs = 30_000) {
  const [now, setNow] = useState(() => Date.now() / 1000);
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now() / 1000), intervalMs);
    return () => clearInterval(id);
  }, [intervalMs]);
  return now;
}

export function ChannelLogo({ ch, className }: { ch: LiveChannel; className?: string }) {
  return (
    <span className={clsx("grid shrink-0 place-items-center overflow-hidden rounded-xl bg-white/[0.06] ring-1 ring-white/10", className)}>
      <Img
        src={ch.logo}
        className="h-full w-full object-contain p-1.5"
        fallback={<span className="font-display text-sm font-bold text-ink-300">{ch.name.split(/\s+/).map((w) => w[0]).join("").slice(0, 3)}</span>}
      />
    </span>
  );
}

function ChannelCard({ ch, now }: { ch: LiveChannel; now: number }) {
  const qc = useQueryClient();
  const p = ch.now;
  const pct = p ? Math.min(100, ((now - p.start) / (p.stop - p.start)) * 100) : 0;
  const fav = async (e: React.MouseEvent) => {
    e.preventDefault();
    await (ch.favorite ? api.del(`/api/livetv/channels/${ch.id}/favorite`) : api.post(`/api/livetv/channels/${ch.id}/favorite`));
    qc.invalidateQueries({ queryKey: ["livetv"] });
  };
  return (
    <Link
      to={`/live/${ch.id}`}
      className="group focus-ring flex flex-col gap-3 rounded-2xl bg-white/[0.04] p-4 ring-1 ring-white/[0.07] transition hover:-translate-y-0.5 hover:bg-white/[0.07] hover:ring-white/20"
    >
      <div className="flex items-center gap-3">
        <ChannelLogo ch={ch} className="h-12 w-16" />
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="text-[12px] font-semibold text-ink-400 tabular-nums">{ch.number}</span>
            <span className="truncate font-semibold">{ch.name}</span>
          </div>
          {ch.group && <div className="truncate text-xs text-ink-500">{ch.group}</div>}
        </div>
        <button onClick={fav} className={clsx("grid h-8 w-8 place-items-center rounded-full transition hover:bg-white/10", ch.favorite ? "text-rose-400" : "text-ink-500 opacity-0 group-hover:opacity-100")}>
          <Heart size={15} fill={ch.favorite ? "currentColor" : "none"} />
        </button>
      </div>
      <div className="min-h-[3.25rem]">
        {p ? (
          <>
            <div className="flex items-baseline justify-between gap-2">
              <span className="truncate text-[15px] font-medium text-white">{p.title}</span>
              <span className="shrink-0 text-xs text-ink-400 tabular-nums">
                {hhmm(p.start)}–{hhmm(p.stop)}
              </span>
            </div>
            <div className="mt-2 h-1 overflow-hidden rounded-full bg-white/10">
              <div className="h-full bg-accent-400" style={{ width: `${pct}%` }} />
            </div>
            {ch.next && (
              <div className="mt-2 truncate text-xs text-ink-400">
                {t("Later")}: {hhmm(ch.next.start)} · {ch.next.title}
              </div>
            )}
          </>
        ) : (
          <div className="text-sm text-ink-500">{t("No guide data")}</div>
        )}
      </div>
    </Link>
  );
}

const SLOT = 30 * 60; // seconds per guide column
const PX_PER_MIN = 6;

function Guide({ channels, now }: { channels: LiveChannel[]; now: number }) {
  const navigate = useNavigate();
  const scroller = useRef<HTMLDivElement>(null);
  const start = Math.floor((now - 3600) / SLOT) * SLOT;
  const end = start + 12 * 3600;
  const width = ((end - start) / 60) * PX_PER_MIN;
  const x = (t: number) => ((Math.max(start, Math.min(end, t)) - start) / 60) * PX_PER_MIN;
  const slots = useMemo(() => Array.from({ length: (end - start) / SLOT }, (_, i) => start + i * SLOT), [start, end]);
  const [sel, setSel] = useState<LiveProgram | null>(null);
  useEffect(() => {
    scroller.current?.scrollTo({ left: x(now) - 160 });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  return (
    <div className="relative">
      <div ref={scroller} className="overflow-x-auto rounded-2xl bg-white/[0.02] ring-1 ring-white/[0.06]">
        <div className="relative" style={{ width: width + 208 }}>
          {/* time header */}
          <div className="sticky top-0 z-20 flex h-10 border-b border-white/[0.06] bg-ink-900/95 backdrop-blur">
            <div className="sticky left-0 z-10 w-52 shrink-0 border-r border-white/[0.06] bg-ink-900" />
            <div className="relative" style={{ width }}>
              {slots.map((s) => (
                <span key={s} className="absolute top-0 flex h-10 items-center border-l border-white/[0.06] pl-2 text-xs text-ink-400 tabular-nums" style={{ left: x(s) }}>
                  {hhmm(s)}
                </span>
              ))}
            </div>
          </div>
          {channels.map((ch) => (
            <div key={ch.id} className="flex h-16 border-b border-white/[0.04]">
              <Link to={`/live/${ch.id}`} className="sticky left-0 z-10 flex w-52 shrink-0 items-center gap-3 border-r border-white/[0.06] bg-ink-900 px-3 hover:bg-ink-850">
                <ChannelLogo ch={ch} className="h-10 w-12" />
                <span className="min-w-0">
                  <span className="block text-[11px] text-ink-500 tabular-nums">{ch.number}</span>
                  <span className="block truncate text-sm font-medium">{ch.name}</span>
                </span>
              </Link>
              <div className="relative" style={{ width }}>
                {(ch.programs ?? []).map((p) => {
                  const live = p.start <= now && p.stop > now;
                  const past = p.stop <= now;
                  return (
                    <button
                      key={p.id}
                      onClick={() => (live ? navigate(`/live/${ch.id}`) : setSel(p))}
                      className={clsx(
                        "absolute top-1.5 bottom-1.5 overflow-hidden rounded-lg border-l-2 px-2.5 text-left transition",
                        live ? "border-accent-400 bg-accent-500/15 hover:bg-accent-500/25" : past ? "border-transparent bg-white/[0.03] text-ink-500" : "border-transparent bg-white/[0.06] hover:bg-white/[0.12]",
                      )}
                      style={{ left: x(p.start) + 2, width: Math.max(0, x(p.stop) - x(p.start) - 4) }}
                      title={p.title}
                    >
                      <span className="block truncate text-[13px] font-medium">{p.title}</span>
                      <span className="block truncate text-[11px] text-ink-400 tabular-nums">
                        {hhmm(p.start)} – {hhmm(p.stop)}
                      </span>
                    </button>
                  );
                })}
              </div>
            </div>
          ))}
          {/* now line */}
          <div className="pointer-events-none absolute top-0 bottom-0 z-10 w-0.5 bg-accent-400 shadow-[0_0_12px] shadow-accent-400" style={{ left: 208 + x(now) }} />
        </div>
      </div>
      {sel && (
        <div className="fixed inset-x-3 bottom-24 z-50 mx-auto max-w-lg animate-fade-up rounded-2xl bg-ink-850 p-5 shadow-2xl ring-1 ring-white/10 sm:bottom-8" onClick={() => setSel(null)}>
          <div className="mb-1 text-xs text-accent-400 tabular-nums">
            {new Date(sel.start * 1000).toLocaleDateString(locale(), { weekday: "long", day: "numeric", month: "long" })} · {hhmm(sel.start)}–{hhmm(sel.stop)}
          </div>
          <div className="font-display text-lg font-semibold">{sel.title}</div>
          {sel.subTitle && <div className="text-sm text-ink-300">{sel.subTitle}</div>}
          {sel.description && <p className="mt-2 text-sm text-ink-300">{sel.description}</p>}
        </div>
      )}
    </div>
  );
}

export function LiveTV() {
  const [sp, setSp] = useSearchParams();
  const tab = sp.get("tab") ?? "channels";
  const group = sp.get("group") ?? "";
  const now = useNow();
  const { data: me } = useMe();
  const { data, isLoading } = useQuery({
    queryKey: ["livetv", tab],
    queryFn: () => api.get<LiveChannel[]>(tab === "guide" ? "/api/livetv/guide?hours=12" : "/api/livetv/channels"),
    refetchInterval: 60_000,
  });
  const groups = useMemo(() => [...new Set((data ?? []).map((c) => c.group).filter(Boolean))] as string[], [data]);
  const shown = (data ?? []).filter((c) => (group === "fav" ? c.favorite : !group || c.group === group));
  const set = (k: string, v: string) => {
    const n = new URLSearchParams(sp);
    if (v) n.set(k, v);
    else n.delete(k);
    setSp(n, { replace: true });
  };
  return (
    <div className="px-5 pt-24 pb-32 sm:px-10 lg:px-14">
      <div className="mb-6 flex flex-wrap items-end justify-between gap-4">
        <h1 className="flex items-center gap-3 font-display text-3xl font-bold tracking-tight sm:text-4xl">
          {t("Live TV")}
          <span className="inline-flex items-center gap-1.5 rounded-full bg-red-500/15 px-2.5 py-1 text-[11px] font-bold tracking-wider text-red-400 uppercase ring-1 ring-red-400/30">
            <Radio size={12} /> Live
          </span>
        </h1>
        <div className="flex gap-2">
          <Chip active={tab === "channels"} onClick={() => set("tab", "")}>
            {t("Channels")}
          </Chip>
          <Chip active={tab === "guide"} onClick={() => set("tab", "guide")}>
            {t("Guide")}
          </Chip>
        </div>
      </div>
      {(groups.length > 0 || data?.some((c) => c.favorite)) && (
        <div className="no-scrollbar -mx-5 mb-6 flex gap-2 overflow-x-auto px-5 sm:mx-0 sm:px-0">
          <Chip active={!group} onClick={() => set("group", "")}>
            {t("All")}
          </Chip>
          {data?.some((c) => c.favorite) && (
            <Chip active={group === "fav"} onClick={() => set("group", "fav")}>
              <Star size={13} className="mr-1 inline -translate-y-px" />
              {t("Favorites")}
            </Chip>
          )}
          {groups.map((g) => (
            <Chip key={g} active={group === g} onClick={() => set("group", g)}>
              {g}
            </Chip>
          ))}
        </div>
      )}
      {isLoading ? (
        <Spinner />
      ) : !data?.length ? (
        <EmptyState icon={<Tv size={28} />} title={t("No channels")}>
          {me?.isAdmin ? (
            <Link to="/admin/livetv" className="text-accent-400 hover:underline">
              {t("Add an M3U playlist in the dashboard.")}
            </Link>
          ) : null}
        </EmptyState>
      ) : tab === "guide" ? (
        <Guide channels={shown} now={now} />
      ) : (
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
          {shown.map((c) => (
            <ChannelCard key={c.id} ch={c} now={now} />
          ))}
        </div>
      )}
    </div>
  );
}
