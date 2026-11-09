import clsx from "clsx";
import { ArrowLeft, Check, ChevronRight } from "lucide-react";
import type { ReactNode } from "react";

export function CtrlButton({ label, onClick, children, active }: { label: string; onClick: () => void; children: ReactNode; active?: boolean }) {
  return (
    <button
      aria-label={label}
      title={label}
      onClick={(e) => {
        e.stopPropagation();
        onClick();
      }}
      className={clsx("focus-ring relative grid h-11 w-11 place-items-center rounded-full transition hover:bg-white/15 active:scale-90", active && "text-accent-400")}
    >
      {children}
      {active && <span className="absolute bottom-1.5 h-0.5 w-4 rounded-full bg-accent-400" />}
    </button>
  );
}

export function MenuPanel({ title, children, onBack }: { title: string; children: ReactNode; onBack?: () => void }) {
  return (
    <div
      className="absolute right-0 bottom-14 max-h-[60vh] w-72 animate-scale-in overflow-y-auto rounded-2xl bg-ink-900/95 p-1.5 shadow-2xl ring-1 ring-white/15 backdrop-blur-xl"
      onClick={(e) => e.stopPropagation()}
    >
      <div className="flex items-center gap-2 px-3 pt-2 pb-2 text-[12px] font-semibold tracking-wider text-ink-400 uppercase">
        {onBack && (
          <button onClick={onBack} className="-ml-1 rounded-full p-1 hover:bg-white/10 hover:text-white">
            <ArrowLeft size={15} />
          </button>
        )}
        {title}
      </div>
      {children}
    </div>
  );
}

export function MenuItem({ active, disabled, onClick, children }: { active?: boolean; disabled?: boolean; onClick: () => void; children: ReactNode }) {
  return (
    <button
      disabled={disabled}
      onClick={onClick}
      className="flex w-full items-center gap-2 rounded-xl px-3 py-2.5 text-left text-sm transition hover:bg-white/10 disabled:opacity-40 disabled:hover:bg-transparent"
    >
      <span className="w-4 shrink-0">{active && <Check size={16} className="text-accent-400" />}</span>
      <span className="min-w-0 flex-1 truncate">{children}</span>
    </button>
  );
}

export function MenuNav({ onClick, value, children }: { onClick: () => void; value: string; children: ReactNode }) {
  return (
    <button onClick={onClick} className="flex w-full items-center gap-3 rounded-xl px-3 py-2.5 text-left text-sm transition hover:bg-white/10">
      <span className="flex-1">{children}</span>
      <span className="max-w-32 truncate text-ink-400">{value}</span>
      <ChevronRight size={16} className="text-ink-500" />
    </button>
  );
}
