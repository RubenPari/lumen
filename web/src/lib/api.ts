import { t } from "./i18n";

export type ItemType = "Movie" | "Series" | "Season" | "Episode";

export interface Person {
  name: string;
  role?: string;
  type: "Actor" | "Director" | "Writer" | string;
  image?: string;
}

export interface Stream {
  index: number;
  type: "video" | "audio" | "subtitle";
  codec: string;
  profile?: string;
  language?: string;
  title?: string;
  channels?: number;
  width?: number;
  height?: number;
  bitRate?: number;
  frameRate?: number;
  pixFmt?: string;
  hdr?: boolean;
  default?: boolean;
  forced?: boolean;
}

export interface UserData {
  position: number;
  played: boolean;
  playCount: number;
  favorite: boolean;
  lastPlayed?: number;
}

export interface Item {
  id: string;
  libraryId: string;
  type: ItemType;
  parentId?: string;
  seriesId?: string;
  seasonId?: string;
  name: string;
  originalTitle?: string;
  sortName?: string;
  path?: string;
  isFolder: boolean;
  year?: number;
  overview?: string;
  tagline?: string;
  premiereDate?: string;
  endDate?: string;
  status?: string;
  communityRating?: number;
  officialRating?: string;
  duration?: number;
  seasonNumber?: number;
  episodeNumber?: number;
  episodeEnd?: number;
  container?: string;
  size?: number;
  genres?: string[];
  studios?: string[];
  people?: Person[];
  tmdbId?: string;
  imdbId?: string;
  media: { container?: string; bitRate?: number; streams?: Stream[]; probed: boolean; chapters?: { start: number; title: string }[] };
  subtitles?: { language?: string; format: string; forced?: boolean }[];
  metadataLocked?: boolean;
  dateAdded: number;
  images: Partial<Record<"primary" | "backdrop" | "logo" | "thumb", string>>;
  parentImages?: Partial<Record<"primary" | "backdrop" | "logo" | "thumb" | "seasonPrimary", string>>;
  userData: UserData;
  seriesName?: string;
  seasonName?: string;
  childCount?: number;
  episodeCount?: number;
  unplayedCount?: number;
  trailers?: string[];
}

export interface Library {
  id: string;
  name: string;
  type: "movies" | "tvshows";
  paths?: string[];
  createdAt: number;
  lastScan: number;
  itemCount: number;
  images: string[];
}

export interface User {
  id: string;
  name: string;
  isAdmin: boolean;
  hasPassword: boolean;
  createdAt: number;
}

export interface SystemInfo {
  serverName: string;
  version: string;
  setupRequired: boolean;
  ffmpeg: boolean;
}

export interface HomeRow {
  id: string;
  title: string;
  kind: "resume" | "nextup" | "latest" | "favorites";
  libraryId?: string;
  items: Item[];
}

export interface SubtitleTrack {
  index: number;
  label: string;
  language?: string;
  codec: string;
  external: boolean;
  forced?: boolean;
  default?: boolean;
  url?: string;
}

export interface AudioTrack {
  index: number;
  label: string;
  language?: string;
  codec: string;
  channels: number;
  default?: boolean;
}

export interface PlaybackInfo {
  item: Item;
  directUrl: string;
  container: string;
  mimeType: string;
  videoCodec: string;
  audioCodec: string;
  width: number;
  height: number;
  hdr: boolean;
  probed: boolean;
  canTranscode: boolean;
  audioTracks: AudioTrack[];
  subtitles: SubtitleTrack[];
  chapters?: { start: number; title: string }[];
  duration: number;
}

export interface ScanStatus {
  running: boolean;
  library?: string;
  phase?: string;
  current: number;
  total: number;
  message?: string;
  lastError?: string;
  finished?: number;
}

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    credentials: "same-origin",
    headers: body !== undefined ? { "Content-Type": "application/json" } : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  const text = await res.text();
  let data: unknown = null;
  try {
    data = text ? JSON.parse(text) : null;
  } catch {
    data = text;
  }
  if (res.ok) localizeNames(data);
  if (!res.ok) {
    const msg = (data && typeof data === "object" && "error" in data ? String((data as { error: string }).error) : res.statusText) || "Request failed";
    throw new ApiError(res.status, msg);
  }
  return data as T;
}

/** Localizes server-generated names ("Season 2", "Episode 3", "Specials") in place. */
function localizeNames(v: unknown, depth = 0): void {
  if (!v || typeof v !== "object" || depth > 6) return;
  if (Array.isArray(v)) {
    for (const x of v) localizeNames(x, depth + 1);
    return;
  }
  const o = v as Record<string, unknown>;
  if (typeof o.name === "string" && (o.type === "Season" || o.type === "Episode")) {
    const m = o.name.match(/^(Season|Episode) (\d+)$/);
    if (m) o.name = m[1] === "Season" ? t("Season {0}", m[2]) : `${t("Episode")} ${m[2]}`;
    else if (o.name === "Specials") o.name = t("Specials");
  }
  if (typeof o.seasonName === "string") {
    const m = o.seasonName.match(/^Season (\d+)$/);
    if (m) o.seasonName = t("Season {0}", m[1]);
    else if (o.seasonName === "Specials") o.seasonName = t("Specials");
  }
  for (const k in o) if (o[k] && typeof o[k] === "object") localizeNames(o[k], depth + 1);
}

export const api = {
  get: <T>(path: string) => request<T>("GET", path),
  post: <T>(path: string, body: unknown = {}) => request<T>("POST", path, body),
  put: <T>(path: string, body: unknown) => request<T>("PUT", path, body),
  patch: <T>(path: string, body: unknown) => request<T>("PATCH", path, body),
  del: <T>(path: string) => request<T>("DELETE", path),
};

export type ImageKind = "primary" | "backdrop" | "logo" | "thumb";

export function imageUrl(id: string, kind: ImageKind, tag?: string, width?: number) {
  const q = new URLSearchParams();
  if (width) q.set("w", String(Math.round(width)));
  if (tag) q.set("tag", tag);
  return `/api/items/${id}/images/${kind}?${q.toString()}`;
}

/** Best landscape image for an item (thumb → backdrop → series backdrop). */
export function landscapeImage(item: Item, width = 640): string | undefined {
  if (item.images.thumb) return imageUrl(item.id, "thumb", item.images.thumb, width);
  if (item.images.backdrop) return imageUrl(item.id, "backdrop", item.images.backdrop, width);
  if (item.seriesId && item.parentImages?.thumb) return imageUrl(item.seriesId, "thumb", item.parentImages.thumb, width);
  if (item.seriesId && item.parentImages?.backdrop) return imageUrl(item.seriesId, "backdrop", item.parentImages.backdrop, width);
  return undefined;
}

/** Best portrait image for an item (primary → season → series poster). */
export function posterImage(item: Item, width = 360): string | undefined {
  if (item.images.primary) return imageUrl(item.id, "primary", item.images.primary, width);
  if (item.seasonId && item.parentImages?.seasonPrimary) return imageUrl(item.seasonId, "primary", item.parentImages.seasonPrimary, width);
  if (item.seriesId && item.parentImages?.primary) return imageUrl(item.seriesId, "primary", item.parentImages.primary, width);
  return undefined;
}

export function backdropImage(item: Item, width = 1920): string | undefined {
  if (item.images.backdrop) return imageUrl(item.id, "backdrop", item.images.backdrop, width);
  if (item.seriesId && item.parentImages?.backdrop) return imageUrl(item.seriesId, "backdrop", item.parentImages.backdrop, width);
  if (item.images.thumb) return imageUrl(item.id, "thumb", item.images.thumb, width);
  return undefined;
}

export function logoImage(item: Item, width = 600): string | undefined {
  if (item.images.logo) return imageUrl(item.id, "logo", item.images.logo, width);
  if (item.seriesId && item.parentImages?.logo) return imageUrl(item.seriesId, "logo", item.parentImages.logo, width);
  return undefined;
}
