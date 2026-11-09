import { useQuery } from "@tanstack/react-query";
import clsx from "clsx";
import { Heart, ListPlus, Play, Shuffle } from "lucide-react";
import { useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { Row } from "../components/cards";
import { MediaCard, MediaHeader, TrackList, totalDuration, usePlay } from "../components/media";
import { Button, Chip, IconButton, Select, Spinner } from "../components/ui";
import { api, imageUrl, posterImage, type Item, type Library } from "../lib/api";
import { trackFromItem, useAudio } from "../lib/audio";
import { useUserDataActions } from "../lib/hooks";
import { t } from "../lib/i18n";
import { ItemGrid } from "./Library";

export function MusicLibrary({ lib }: { lib: Library }) {
  const [sp, setSp] = useSearchParams();
  const tab = sp.get("tab") ?? "albums";
  const sort = sp.get("sort") ?? (tab === "albums" ? "dateAdded" : "name");
  const set = (k: string, v: string) => {
    const n = new URLSearchParams(sp);
    n.set(k, v);
    if (k === "tab") n.delete("sort");
    setSp(n, { replace: true });
  };
  const type = { albums: "MusicAlbum", artists: "MusicArtist", songs: "Audio" }[tab] ?? "MusicAlbum";
  const params = new URLSearchParams({ libraryId: lib.id, type, sort, order: sort === "name" || sort === "artist" ? "asc" : "desc" });
  const audio = useAudio();
  const shuffleAll = async () => {
    const r = await api.get<{ items: Item[] }>(`/api/items?libraryId=${lib.id}&type=Audio&sort=random&limit=300`);
    audio.playTracks(r.items.map((i) => trackFromItem(i)));
  };
  const header = (
    <div className="mb-6 space-y-5">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <h1 className="font-display text-3xl font-bold tracking-tight sm:text-4xl">{lib.name}</h1>
        <div className="flex items-center gap-2">
          {tab !== "artists" && (
            <Select
              value={sort}
              onChange={(v) => set("sort", v)}
              options={[
                { value: "dateAdded", label: t("Date added") },
                { value: "name", label: t("Name") },
                ...(tab === "albums" ? [{ value: "artist", label: t("Artist") }, { value: "year", label: t("Release year") }] : []),
                { value: "lastPlayed", label: t("Last played") },
                { value: "random", label: t("Random") },
              ]}
            />
          )}
          <Button variant="primary" onClick={shuffleAll}>
            <Shuffle size={17} /> {t("Shuffle")}
          </Button>
        </div>
      </div>
      <div className="flex gap-2">
        {[
          ["albums", t("Albums")],
          ["artists", t("Artists")],
          ["songs", t("Songs")],
        ].map(([v, l]) => (
          <Chip key={v} active={tab === v} onClick={() => set("tab", v)}>
            {l}
          </Chip>
        ))}
      </div>
    </div>
  );
  return (
    <div className="px-5 pt-24 pb-32 sm:px-10 lg:px-14">
      {tab === "songs" ? <Songs params={params} header={header} /> : <ItemGrid key={tab} params={params} header={header} renderItem={(it) => <MediaCard key={it.id} item={it} />} />}
    </div>
  );
}

function Songs({ params, header }: { params: URLSearchParams; header: React.ReactNode }) {
  const p = new URLSearchParams(params);
  p.set("limit", "500");
  const { data, isLoading } = useQuery({ queryKey: ["items", "songs", p.toString()], queryFn: () => api.get<{ items: Item[] }>(`/api/items?${p}`) });
  return (
    <>
      {header}
      {isLoading ? <Spinner /> : <TrackList tracks={data?.items ?? []} showAlbum />}
    </>
  );
}

export function AlbumView({ album }: { album: Item }) {
  const { data: tracks } = useQuery({ queryKey: ["children", album.id], queryFn: () => api.get<Item[]>(`/api/items/${album.id}/children`) });
  const { data: more } = useQuery({
    queryKey: ["children", album.artistId],
    queryFn: () => api.get<Item[]>(`/api/items/${album.artistId}/children`),
    enabled: !!album.artistId,
  });
  const audio = useAudio();
  const { setFavorite } = useUserDataActions();
  const list = tracks ?? [];
  const others = (more ?? []).filter((a) => a.id !== album.id);
  return (
    <div className="pb-32">
      <MediaHeader
        item={album}
        image={posterImage(album, 600)}
        kicker={t("Album")}
        meta={
          <>
            <Link to={`/item/${album.artistId}`} className="font-semibold text-white hover:underline">
              {album.albumArtist}
            </Link>
            {[album.year, album.genres?.slice(0, 2).join(", "), t("{0} songs", list.length), totalDuration(list)]
              .filter(Boolean)
              .map((x) => ` · ${x}`)
              .join("")}
          </>
        }
        actions={
          <>
            <Button variant="primary" size="lg" onClick={() => audio.playTracks(list.map((tr) => trackFromItem(tr, album)))}>
              <Play size={19} fill="currentColor" /> {t("Play")}
            </Button>
            <Button
              variant="glass"
              size="lg"
              onClick={() => {
                audio.setShuffle(true);
                audio.playTracks(list.map((tr) => trackFromItem(tr, album)), Math.floor(Math.random() * list.length));
              }}
            >
              <Shuffle size={19} /> {t("Shuffle")}
            </Button>
            <IconButton label={t("Add to queue")} className="h-12 w-12" onClick={() => audio.enqueue(list.map((tr) => trackFromItem(tr, album)))}>
              <ListPlus size={19} />
            </IconButton>
            <IconButton
              label={t("Favorite")}
              className={clsx("h-12 w-12", album.userData.favorite && "!bg-rose-500/20 !text-rose-400")}
              onClick={() => setFavorite(album.id, !album.userData.favorite)}
            >
              <Heart size={19} fill={album.userData.favorite ? "currentColor" : "none"} />
            </IconButton>
          </>
        }
      />
      <div className="mx-auto max-w-5xl px-3 sm:px-8">{tracks ? <TrackList tracks={list} album={album} /> : <Spinner />}</div>
      {others.length > 0 && (
        <div className="mt-12">
          <Row title={t("More by {0}", album.albumArtist ?? "")}>
            {others.map((a) => (
              <MediaCard key={a.id} item={a} sub={a.year ? String(a.year) : ""} />
            ))}
          </Row>
        </div>
      )}
    </div>
  );
}

export function ArtistView({ artist }: { artist: Item }) {
  const { data: albums } = useQuery({ queryKey: ["children", artist.id], queryFn: () => api.get<Item[]>(`/api/items/${artist.id}/children`) });
  const { data: top } = useQuery({
    queryKey: ["items", "artist-tracks", artist.id],
    queryFn: () => api.get<{ items: Item[] }>(`/api/items?type=Audio&artistId=${artist.id}&sort=lastPlayed&order=desc&limit=10`),
  });
  const play = usePlay();
  const [all, setAll] = useState(false);
  const image = artist.images.primary ? imageUrl(artist.id, "primary", artist.images.primary, 600) : artist.images.thumb ? imageUrl(artist.id, "thumb", artist.images.thumb, 600) : undefined;
  const tracks = top?.items ?? [];
  return (
    <div className="pb-32">
      <MediaHeader
        item={artist}
        image={image}
        round
        kicker={t("Artist")}
        meta={t("{0} albums", albums?.length ?? 0)}
        actions={
          <Button variant="primary" size="lg" onClick={() => play(artist)}>
            <Shuffle size={19} /> {t("Shuffle")}
          </Button>
        }
      />
      {artist.overview && <p className="mx-auto mb-8 max-w-3xl px-5 text-ink-300">{artist.overview}</p>}
      {tracks.length > 0 && (
        <div className="mx-auto mb-10 max-w-5xl px-3 sm:px-8">
          <h2 className="mb-3 px-3 font-display text-xl font-semibold">{t("Songs")}</h2>
          <TrackList tracks={all ? tracks : tracks.slice(0, 5)} showAlbum />
          {tracks.length > 5 && (
            <Button variant="ghost" size="sm" className="mt-2" onClick={() => setAll((a) => !a)}>
              {all ? t("Show less") : t("Show more")}
            </Button>
          )}
        </div>
      )}
      <div className="px-5 sm:px-10 lg:px-14">
        <h2 className="mb-4 font-display text-xl font-semibold">{t("Albums")}</h2>
        <div className="grid grid-cols-2 gap-x-4 gap-y-6 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6">
          {albums?.map((a) => (
            <MediaCard key={a.id} item={a} sub={a.year ? String(a.year) : ""} />
          ))}
        </div>
      </div>
    </div>
  );
}
