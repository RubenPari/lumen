import type { PlaybackInfo } from "./api";

const videoCodecs: Record<string, string> = {
  h264: "avc1.640028",
  hevc: "hvc1.1.6.L120.90",
  vp9: "vp09.00.40.08",
  vp8: "vp8",
  av1: "av01.0.08M.08",
};

const audioCodecs: Record<string, string> = {
  aac: "mp4a.40.2",
  mp3: "mp4a.69",
  opus: "opus",
  flac: "flac",
  ac3: "ac-3",
  eac3: "ec-3",
  vorbis: "vorbis",
};

function supported(mime: string): boolean {
  try {
    if (typeof MediaSource !== "undefined" && MediaSource.isTypeSupported(mime)) return true;
  } catch {
    /* ignore */
  }
  const v = document.createElement("video");
  return v.canPlayType(mime) === "probably";
}

const isChromium = () => /Chrome\//.test(navigator.userAgent);
const isFirefox = () => /Firefox\//.test(navigator.userAgent);

/** Decides whether the browser can play the file without transcoding. */
export function canDirectPlay(info: PlaybackInfo): { ok: boolean; reason?: string } {
  const c = (info.container || "").toLowerCase();
  const containerOk = ["mp4", "m4v", "mov", "webm"].includes(c) || (c === "mkv" && (isChromium() || isFirefox()));
  if (!containerOk) return { ok: false, reason: `container ${c}` };
  if (!info.probed) return { ok: true }; // optimistic: let the browser try
  const v = videoCodecs[info.videoCodec];
  if (info.videoCodec && !v) return { ok: false, reason: `video ${info.videoCodec}` };
  const item = info.item;
  const vs = item.media?.streams?.find((s) => s.type === "video");
  if (vs?.pixFmt?.includes("10") && info.videoCodec === "h264") return { ok: false, reason: "10-bit h264" };
  const audio = item.media?.streams?.find((s) => s.type === "audio" && s.default) ?? item.media?.streams?.find((s) => s.type === "audio");
  const a = audio ? audioCodecs[audio.codec] : undefined;
  if (audio && !a) return { ok: false, reason: `audio ${audio.codec}` };
  const mime = c === "webm" ? "video/webm" : "video/mp4";
  const codecs = [v, a].filter(Boolean).join(",");
  if (codecs && !supported(`${mime}; codecs="${codecs}"`)) return { ok: false, reason: `codecs ${codecs}` };
  return { ok: true };
}

export const qualities = [
  { id: "original", label: "Original", maxHeight: 0, maxBitrate: 0 },
  { id: "1080", label: "1080p · 10 Mbps", maxHeight: 1080, maxBitrate: 10000 },
  { id: "720", label: "720p · 4 Mbps", maxHeight: 720, maxBitrate: 4000 },
  { id: "480", label: "480p · 1.5 Mbps", maxHeight: 480, maxBitrate: 1500 },
] as const;

export type QualityId = (typeof qualities)[number]["id"];
