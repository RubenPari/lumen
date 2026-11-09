import { t, locale } from "./i18n";
import type { Item } from "./api";

export function formatDuration(sec?: number): string {
  if (!sec || sec <= 0) return "";
  const m = Math.round(sec / 60);
  if (m < 60) return `${m}m`;
  const h = Math.floor(m / 60);
  const r = m % 60;
  return r ? `${h}h ${r}m` : `${h}h`;
}

export function formatClock(sec: number): string {
  if (!isFinite(sec) || sec < 0) sec = 0;
  const h = Math.floor(sec / 3600);
  const m = Math.floor((sec % 3600) / 60);
  const s = Math.floor(sec % 60);
  const mm = h ? String(m).padStart(2, "0") : String(m);
  return `${h ? h + ":" : ""}${mm}:${String(s).padStart(2, "0")}`;
}

export function endsAt(remainingSec: number): string {
  const d = new Date(Date.now() + remainingSec * 1000);
  return d.toLocaleTimeString(locale(), { hour: "2-digit", minute: "2-digit" });
}

export function formatBytes(n: number): string {
  if (!n) return "0 B";
  const u = ["B", "KB", "MB", "GB", "TB", "PB"];
  const i = Math.min(u.length - 1, Math.floor(Math.log(n) / Math.log(1024)));
  return `${(n / Math.pow(1024, i)).toFixed(i > 2 ? 1 : 0)} ${u[i]}`;
}

export function formatRelative(unix?: number): string {
  if (!unix) return t("never");
  const diff = Date.now() / 1000 - unix;
  const rtf = new Intl.RelativeTimeFormat(locale(), { numeric: "auto" });
  if (diff < 60) return rtf.format(-Math.round(diff), "second");
  if (diff < 3600) return rtf.format(-Math.round(diff / 60), "minute");
  if (diff < 86400) return rtf.format(-Math.round(diff / 3600), "hour");
  return rtf.format(-Math.round(diff / 86400), "day");
}

export function episodeLabel(it: Item): string {
  if (it.type !== "Episode") return "";
  const s = it.seasonNumber ?? 1;
  const e = it.episodeNumber;
  if (e == null) return s === 0 ? t("Specials") : t("Season {0}", s);
  const end = it.episodeEnd && it.episodeEnd !== e ? `-${it.episodeEnd}` : "";
  return `S${s}:E${e}${end}`;
}

export function progressPct(it: Item): number {
  if (!it.duration || !it.userData?.position) return 0;
  return Math.min(100, (it.userData.position / it.duration) * 100);
}

export function resolutionLabel(w?: number, h?: number): string | undefined {
  if (!w || !h) return undefined;
  if (w >= 3800 || h >= 2100) return "4K";
  if (w >= 2500 || h >= 1400) return "1440p";
  if (w >= 1900 || h >= 1000) return "1080p";
  if (w >= 1200 || h >= 700) return "720p";
  if (h >= 560) return "576p";
  return "SD";
}

export function channelsLabel(c?: number): string | undefined {
  if (!c) return undefined;
  if (c === 1) return "Mono";
  if (c === 2) return "Stereo";
  if (c === 6) return "5.1";
  if (c === 8) return "7.1";
  return `${c}ch`;
}

export function yearRange(it: Item): string {
  if (!it.year) return "";
  if (it.type !== "Series") return String(it.year);
  const end = it.endDate?.slice(0, 4);
  if (end && end !== String(it.year)) return `${it.year} – ${end}`;
  if (it.status === "Returning Series" || it.status === "Continuing") return `${it.year} –`;
  return String(it.year);
}

export const isMac = typeof navigator !== "undefined" && /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent);
