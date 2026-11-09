export interface Cue {
  start: number;
  end: number;
  html: string;
}

function parseTime(s: string): number {
  const m = s.trim().match(/(?:(\d+):)?(\d{1,2}):(\d{2})[.,](\d{1,3})/);
  if (!m) return NaN;
  const [, h, mm, ss, ms] = m;
  return (h ? parseInt(h) * 3600 : 0) + parseInt(mm) * 60 + parseInt(ss) + parseInt(ms.padEnd(3, "0")) / 1000;
}

function escapeHtml(s: string) {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

/** Escapes text and re-allows a few harmless formatting tags. */
function sanitize(text: string): string {
  return escapeHtml(text)
    .replace(/&lt;(\/?)(i|b|u)&gt;/gi, "<$1$2>")
    .replace(/&lt;\/?[^&]*&gt;/g, "")
    .replace(/\n/g, "<br/>");
}

export function parseVTT(src: string): Cue[] {
  const cues: Cue[] = [];
  const blocks = src.replace(/\r\n?/g, "\n").split(/\n{2,}/);
  for (const block of blocks) {
    const lines = block.split("\n");
    const i = lines.findIndex((l) => l.includes("-->"));
    if (i < 0) continue;
    const [a, b] = lines[i].split("-->");
    const start = parseTime(a);
    const end = parseTime(b.trim().split(/\s+/)[0]);
    if (isNaN(start) || isNaN(end)) continue;
    const text = lines.slice(i + 1).join("\n").trim();
    if (!text) continue;
    cues.push({ start, end, html: sanitize(text) });
  }
  cues.sort((x, y) => x.start - y.start);
  return cues;
}

/** Returns the cues active at time t (binary search on start). */
export function activeCues(cues: Cue[], t: number): Cue[] {
  let lo = 0,
    hi = cues.length;
  while (lo < hi) {
    const mid = (lo + hi) >> 1;
    if (cues[mid].start <= t) lo = mid + 1;
    else hi = mid;
  }
  const out: Cue[] = [];
  for (let i = lo - 1; i >= 0 && i >= lo - 20; i--) {
    if (cues[i].end > t) out.unshift(cues[i]);
  }
  return out;
}
