import clsx from "clsx";
import { ListMusic, Music2, Pause, Play, Repeat, Repeat1, Shuffle, SkipBack, SkipForward, Volume2, VolumeX, X } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router-dom";
import { useAudio } from "../lib/audio";
import { formatClock } from "../lib/format";
import { t } from "../lib/i18n";
import { Img } from "./cards";

export function MiniPlayer() {
  const a = useAudio();
  const [queueOpen, setQueueOpen] = useState(false);
  const [scrub, setScrub] = useState<number | null>(null);
  if (!a.current) return null;
  const c = a.current;
  const dur = a.duration || c.duration;
  const shown = scrub ?? a.time;
  const pct = dur ? (shown / dur) * 100 : 0;
  const link = c.bookId ? `/item/${c.bookId}` : c.albumId ? `/item/${c.albumId}` : undefined;
  const RepeatIcon = a.repeat === "one" ? Repeat1 : Repeat;

  return (
    <>
      {queueOpen && (
        <div className="glass fixed right-3 bottom-[calc(6rem+env(safe-area-inset-bottom))] z-[60] max-h-[60vh] w-[min(380px,calc(100vw-1.5rem))] animate-scale-in overflow-y-auto rounded-3xl p-2 shadow-2xl ring-1 ring-white/10 sm:right-6 sm:bottom-28">
          <div className="px-3 pt-2 pb-2 text-[12px] font-semibold tracking-wider text-ink-400 uppercase">{t("Queue")}</div>
          {a.queue.map((tr, i) => (
            <div
              key={tr.id + i}
              className={clsx("group flex items-center gap-3 rounded-2xl px-2 py-2", i === a.index ? "bg-white/10" : "hover:bg-white/5")}
            >
              <button onClick={() => a.jump(i)} className="flex min-w-0 flex-1 items-center gap-3 text-left">
                <span className="relative h-10 w-10 shrink-0 overflow-hidden rounded-lg bg-ink-700">
                  <Img src={tr.image} className="h-full w-full object-cover" fallback={<Music2 size={16} className="m-3 text-ink-400" />} />
                </span>
                <span className="min-w-0">
                  <span className={clsx("block truncate text-sm", i === a.index ? "font-semibold text-accent-300" : "text-white")}>{tr.name}</span>
                  <span className="block truncate text-xs text-ink-400">{tr.artist}</span>
                </span>
              </button>
              <span className="text-xs text-ink-500 tabular-nums">{formatClock(tr.duration)}</span>
              {i !== a.index && (
                <button onClick={() => a.removeAt(i)} className="text-ink-500 opacity-0 transition group-hover:opacity-100 hover:text-white" aria-label="Remove">
                  <X size={15} />
                </button>
              )}
            </div>
          ))}
        </div>
      )}
      <div className="fixed inset-x-0 bottom-[calc(4rem+env(safe-area-inset-bottom))] z-[55] px-2 pb-2 sm:bottom-0 sm:px-4 sm:pb-4">
        <div className="glass mx-auto flex max-w-6xl animate-fade-up flex-col overflow-hidden rounded-3xl shadow-2xl shadow-black/60 ring-1 ring-white/10">
          {/* Progress */}
          <div
            className="group relative h-1.5 cursor-pointer bg-white/10"
            onPointerDown={(e) => {
              const r = e.currentTarget.getBoundingClientRect();
              const v = ((e.clientX - r.left) / r.width) * dur;
              setScrub(v);
              a.seek(v);
              setTimeout(() => setScrub(null), 300);
            }}
          >
            <div className="h-full bg-accent-400 transition-[width] duration-200" style={{ width: `${pct}%` }} />
          </div>
          <div className="flex items-center gap-3 p-2.5 sm:gap-4 sm:px-4">
            <Link to={link ?? "#"} className="flex min-w-0 flex-1 items-center gap-3">
              <span className="relative h-12 w-12 shrink-0 overflow-hidden rounded-xl bg-ink-700 shadow-lg">
                <Img src={c.image} className="h-full w-full object-cover" fallback={<Music2 size={20} className="m-3.5 text-ink-400" />} />
              </span>
              <span className="min-w-0">
                <span className="block truncate text-[14px] font-semibold text-white">{c.name}</span>
                <span className="block truncate text-[12px] text-ink-400">
                  {[c.artist, c.album].filter(Boolean).join(" · ")}
                </span>
              </span>
            </Link>
            <div className="hidden text-xs text-ink-400 tabular-nums md:block">
              {formatClock(shown)} / {formatClock(dur)}
            </div>
            <div className="flex items-center gap-0.5 sm:gap-1">
              <SmallBtn label={t("Shuffle")} onClick={() => a.setShuffle(!a.shuffle)} active={a.shuffle} className="hidden sm:grid">
                <Shuffle size={17} />
              </SmallBtn>
              <SmallBtn label="Previous" onClick={a.prev} className="hidden sm:grid">
                <SkipBack size={19} fill="currentColor" />
              </SmallBtn>
              <button
                onClick={a.toggle}
                aria-label={a.playing ? "Pause" : t("Play")}
                className={clsx("grid h-11 w-11 place-items-center rounded-full bg-white text-ink-950 shadow-lg transition active:scale-90", a.buffering && "animate-pulse")}
              >
                {a.playing ? <Pause size={20} fill="currentColor" /> : <Play size={20} fill="currentColor" className="ml-0.5" />}
              </button>
              <SmallBtn label={t("Next")} onClick={a.next}>
                <SkipForward size={19} fill="currentColor" />
              </SmallBtn>
              <SmallBtn label="Repeat" onClick={() => a.setRepeat(a.repeat === "off" ? "all" : a.repeat === "all" ? "one" : "off")} active={a.repeat !== "off"} className="hidden sm:grid">
                <RepeatIcon size={17} />
              </SmallBtn>
            </div>
            <div className="hidden items-center gap-2 lg:flex">
              <SmallBtn label="Mute" onClick={() => a.setVolume(a.volume > 0 ? 0 : 1)}>
                {a.volume === 0 ? <VolumeX size={18} /> : <Volume2 size={18} />}
              </SmallBtn>
              <input
                type="range"
                min={0}
                max={1}
                step={0.01}
                value={a.volume}
                onChange={(e) => a.setVolume(parseFloat(e.target.value))}
                className="slider w-24"
                style={{ ["--value" as string]: `${a.volume * 100}%` }}
              />
            </div>
            <SmallBtn label={t("Queue")} onClick={() => setQueueOpen((o) => !o)} active={queueOpen}>
              <ListMusic size={18} />
            </SmallBtn>
            <SmallBtn label={t("Close")} onClick={a.close} className="hidden sm:grid">
              <X size={18} />
            </SmallBtn>
          </div>
        </div>
      </div>
    </>
  );
}

function SmallBtn({ label, onClick, children, active, className }: { label: string; onClick: () => void; children: React.ReactNode; active?: boolean; className?: string }) {
  return (
    <button
      aria-label={label}
      title={label}
      onClick={onClick}
      className={clsx("grid h-10 w-10 place-items-center rounded-full transition hover:bg-white/10 active:scale-90", active ? "text-accent-400" : "text-ink-200", className)}
    >
      {children}
    </button>
  );
}
