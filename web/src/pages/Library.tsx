import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { ArrowDownWideNarrow, ArrowUpNarrowWide, Film, Shuffle } from "lucide-react";
import { useEffect } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";
import { Grid, PosterCard, SkeletonCard } from "../components/cards";
import { Chip, EmptyState, IconButton, Select } from "../components/ui";
import { api, type Item } from "../lib/api";
import { useInView, useLibraries } from "../lib/hooks";
import { t } from "../lib/i18n";

const PAGE = 60;

function ItemGrid({ params, header }: { params: URLSearchParams; header: React.ReactNode }) {
  const [ref, inView] = useInView<HTMLDivElement>();
  const key = params.toString();
  const q = useInfiniteQuery({
    queryKey: ["items", key],
    initialPageParam: 0,
    queryFn: ({ pageParam }) => {
      const p = new URLSearchParams(params);
      p.set("limit", String(PAGE));
      p.set("offset", String(pageParam));
      return api.get<{ items: Item[]; total: number }>(`/api/items?${p}`);
    },
    getNextPageParam: (last, all) => {
      const n = all.reduce((a, p) => a + p.items.length, 0);
      return n < last.total ? n : undefined;
    },
  });
  useEffect(() => {
    if (inView && q.hasNextPage && !q.isFetchingNextPage) q.fetchNextPage();
  }, [inView, q]);
  const items = q.data?.pages.flatMap((p) => p.items) ?? [];
  const total = q.data?.pages[0]?.total ?? 0;
  return (
    <>
      {header}
      <div className="mb-5 text-[13px] text-ink-400">{q.isSuccess && t("{0} items", total)}</div>
      {q.isLoading ? (
        <Grid>
          {Array.from({ length: 18 }).map((_, i) => (
            <SkeletonCard key={i} />
          ))}
        </Grid>
      ) : items.length === 0 ? (
        <EmptyState icon={<Film size={28} />} title={t("Nothing here yet")} />
      ) : (
        <Grid>
          {items.map((it) => (
            <PosterCard key={it.id} item={it} />
          ))}
        </Grid>
      )}
      <div ref={ref} className="h-10" />
    </>
  );
}

const LETTERS = ["#", ..."ABCDEFGHIJKLMNOPQRSTUVWXYZ"];

export function Library() {
  const { id } = useParams();
  const { data: libs } = useLibraries();
  const lib = libs?.find((l) => l.id === id);
  const [sp, setSp] = useSearchParams();
  const navigate = useNavigate();
  const sort = sp.get("sort") ?? "name";
  const order = sp.get("order") ?? (sort === "name" ? "asc" : "desc");
  const filter = sp.get("filter") ?? "";
  const genre = sp.get("genre") ?? "";
  const letter = sp.get("letter") ?? "";

  const { data: genres } = useQuery({
    queryKey: ["genres", id],
    queryFn: () => api.get<{ name: string; count: number }[]>(`/api/genres?libraryId=${id}`),
    enabled: !!id,
  });

  const set = (k: string, v: string) => {
    const n = new URLSearchParams(sp);
    if (v) n.set(k, v);
    else n.delete(k);
    if (k === "sort") n.delete("order");
    setSp(n, { replace: true });
  };

  const params = new URLSearchParams({ libraryId: id ?? "", sort, order });
  if (filter) params.set("filter", filter);
  if (genre) params.set("genre", genre);
  if (letter) params.set("letter", letter);

  const shuffle = async () => {
    const r = await api.get<{ items: Item[] }>(`/api/items?libraryId=${id}&sort=random&limit=1`);
    if (r.items[0]) navigate(`/item/${r.items[0].id}`);
  };

  const header = (
    <div className="mb-6 space-y-5">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <h1 className="font-display text-3xl font-bold tracking-tight sm:text-4xl">{lib?.name ?? ""}</h1>
        <div className="flex flex-wrap items-center gap-2">
          <Select
            value={sort}
            onChange={(v) => set("sort", v)}
            options={[
              { value: "name", label: t("Name") },
              { value: "dateAdded", label: t("Date added") },
              { value: "year", label: t("Release year") },
              { value: "rating", label: t("Rating") },
              { value: "runtime", label: t("Runtime") },
              { value: "lastPlayed", label: t("Last played") },
              { value: "random", label: t("Random") },
            ]}
          />
          {sort !== "random" && (
            <IconButton label={order === "asc" ? "Ascending" : "Descending"} onClick={() => set("order", order === "asc" ? "desc" : "asc")}>
              {order === "asc" ? <ArrowUpNarrowWide size={18} /> : <ArrowDownWideNarrow size={18} />}
            </IconButton>
          )}
          <IconButton label={t("Shuffle")} onClick={shuffle}>
            <Shuffle size={18} />
          </IconButton>
        </div>
      </div>
      <div className="no-scrollbar -mx-5 flex gap-2 overflow-x-auto px-5 sm:mx-0 sm:px-0">
        {[
          ["", t("All")],
          ["unplayed", t("Unwatched")],
          ["resumable", t("In progress")],
          ["favorites", t("Favorites")],
        ].map(([v, l]) => (
          <Chip key={v} active={filter === v} onClick={() => set("filter", v)}>
            {l}
          </Chip>
        ))}
        {genres && genres.length > 0 && (
          <>
            <span className="mx-1 w-px shrink-0 bg-white/10" />
            <Select
              value={genre}
              onChange={(v) => set("genre", v)}
              className="h-9 text-[13px]"
              options={[{ value: "", label: t("All genres") }, ...genres.map((g) => ({ value: g.name, label: `${g.name} (${g.count})` }))]}
            />
          </>
        )}
      </div>
      {sort === "name" && (
        <div className="no-scrollbar -mx-5 flex gap-0.5 overflow-x-auto px-5 sm:mx-0 sm:px-0">
          {LETTERS.map((l) => (
            <button
              key={l}
              onClick={() => set("letter", letter === l ? "" : l)}
              className={`h-7 min-w-7 shrink-0 rounded-md text-[12px] font-semibold transition ${letter === l ? "bg-accent-500 text-ink-950" : "text-ink-400 hover:bg-white/10 hover:text-white"}`}
            >
              {l}
            </button>
          ))}
        </div>
      )}
    </div>
  );

  return (
    <div className="px-5 pt-24 pb-16 sm:px-10 lg:px-14">
      <ItemGrid params={params} header={header} />
    </div>
  );
}

/** Generic browse page: /browse?person=…, /browse?genre=…, /browse?studio=… */
export function Browse() {
  const [sp] = useSearchParams();
  const person = sp.get("person");
  const genre = sp.get("genre");
  const studio = sp.get("studio");
  const title = person ?? genre ?? studio ?? "";
  const params = new URLSearchParams({ sort: "year", order: "desc" });
  if (person) params.set("person", person);
  if (genre) params.set("genre", genre);
  if (studio) params.set("studio", studio);
  return (
    <div className="px-5 pt-24 pb-16 sm:px-10 lg:px-14">
      <ItemGrid params={params} header={<h1 className="mb-2 font-display text-3xl font-bold tracking-tight sm:text-4xl">{title}</h1>} />
    </div>
  );
}
