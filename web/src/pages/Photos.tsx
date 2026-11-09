import { useQuery } from "@tanstack/react-query";
import { Images, Play } from "lucide-react";
import { useState } from "react";
import { Lightbox, MediaCard, PhotoGrid, groupByMonth } from "../components/media";
import { Button, Chip, EmptyState, Spinner } from "../components/ui";
import { api, type Item, type Library } from "../lib/api";
import { t } from "../lib/i18n";
import { useSearchParams } from "react-router-dom";

function Timeline({ photos }: { photos: Item[] }) {
  const [open, setOpen] = useState<number | null>(null);
  const groups = groupByMonth(photos);
  let base = 0;
  return (
    <>
      <div className="space-y-8">
        {groups.map((g) => {
          const start = base;
          base += g.items.length;
          return (
            <section key={g.key}>
              <h2 className="sticky top-16 z-10 -mx-1 mb-3 bg-ink-950/80 px-1 py-2 font-display text-lg font-semibold capitalize backdrop-blur">{g.label}</h2>
              <PhotoGrid photos={g.items} onOpen={(i) => setOpen(start + i)} />
            </section>
          );
        })}
      </div>
      {open !== null && <Lightbox photos={photos} index={open} onClose={() => setOpen(null)} />}
    </>
  );
}

export function PhotoLibrary({ lib }: { lib: Library }) {
  const [sp, setSp] = useSearchParams();
  const tab = sp.get("tab") ?? "timeline";
  const { data: photos, isLoading } = useQuery({
    queryKey: ["items", "photos", lib.id],
    queryFn: () => api.get<{ items: Item[] }>(`/api/items?libraryId=${lib.id}&type=Photo&sort=taken&order=desc&limit=1000`),
    enabled: tab === "timeline",
  });
  const { data: albums } = useQuery({
    queryKey: ["items", "photo-albums", lib.id],
    queryFn: () => api.get<{ items: Item[] }>(`/api/items?libraryId=${lib.id}&type=PhotoAlbum&root=true&sort=taken&order=desc&limit=500`),
    enabled: tab === "albums",
  });
  return (
    <div className="px-5 pt-24 pb-32 sm:px-10 lg:px-14">
      <div className="mb-6 flex flex-wrap items-end justify-between gap-4">
        <h1 className="font-display text-3xl font-bold tracking-tight sm:text-4xl">{lib.name}</h1>
        <div className="flex gap-2">
          <Chip active={tab === "timeline"} onClick={() => setSp({ tab: "timeline" }, { replace: true })}>
            {t("Timeline")}
          </Chip>
          <Chip active={tab === "albums"} onClick={() => setSp({ tab: "albums" }, { replace: true })}>
            {t("Albums")}
          </Chip>
        </div>
      </div>
      {tab === "timeline" ? (
        isLoading ? (
          <Spinner />
        ) : photos?.items.length ? (
          <Timeline photos={photos.items} />
        ) : (
          <EmptyState icon={<Images size={28} />} title={t("Nothing here yet")} />
        )
      ) : (
        <div className="grid grid-cols-2 gap-x-4 gap-y-6 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6">
          {albums?.items.map((a) => (
            <MediaCard key={a.id} item={a} />
          ))}
        </div>
      )}
    </div>
  );
}

export function PhotoAlbumView({ album }: { album: Item }) {
  const { data: kids } = useQuery({ queryKey: ["children", album.id], queryFn: () => api.get<Item[]>(`/api/items/${album.id}/children`) });
  const [open, setOpen] = useState<number | null>(null);
  const sub = (kids ?? []).filter((k) => k.type === "PhotoAlbum");
  const photos = (kids ?? []).filter((k) => k.type === "Photo");
  return (
    <div className="px-5 pt-24 pb-32 sm:px-10 lg:px-14">
      <div className="mb-6 flex flex-wrap items-end justify-between gap-4">
        <div>
          <div className="mb-1 text-[12px] font-semibold tracking-[0.18em] text-accent-400 uppercase">{t("Album")}</div>
          <h1 className="font-display text-3xl font-bold tracking-tight sm:text-4xl">{album.name}</h1>
          <div className="mt-1 text-ink-400">{t("{0} photos", photos.length)}</div>
        </div>
        {photos.length > 0 && (
          <Button variant="primary" onClick={() => setOpen(0)}>
            <Play size={17} fill="currentColor" /> {t("Slideshow")}
          </Button>
        )}
      </div>
      {!kids && <Spinner />}
      {sub.length > 0 && (
        <div className="mb-8 grid grid-cols-2 gap-x-4 gap-y-6 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6">
          {sub.map((a) => (
            <MediaCard key={a.id} item={a} />
          ))}
        </div>
      )}
      <PhotoGrid photos={photos} onOpen={setOpen} />
      {open !== null && <Lightbox photos={photos} index={open} onClose={() => setOpen(null)} />}
    </div>
  );
}
