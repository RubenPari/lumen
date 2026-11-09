import clsx from "clsx";
import type { ReactNode } from "react";
import { Spinner } from "../../components/ui";
import type { ScanStatus } from "../../lib/api";
import { t } from "../../lib/i18n";

export function Card({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={clsx("rounded-3xl bg-white/[0.035] p-5 ring-1 ring-white/[0.07] sm:p-6", className)}>{children}</div>;
}

export function Stat({ icon, label, value }: { icon: ReactNode; label: string; value: ReactNode }) {
  return (
    <Card className="flex items-center gap-4">
      <div className="grid h-12 w-12 shrink-0 place-items-center rounded-2xl bg-accent-500/10 text-accent-400 ring-1 ring-accent-400/20">{icon}</div>
      <div className="min-w-0">
        <div className="font-display text-2xl font-semibold tabular-nums">{value}</div>
        <div className="truncate text-[13px] text-ink-400">{label}</div>
      </div>
    </Card>
  );
}

export function ScanBar({ scan }: { scan: ScanStatus }) {
  if (!scan.running) return null;
  const pct = scan.total ? (scan.current / scan.total) * 100 : 0;
  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between gap-3 text-[13px]">
        <span className="flex min-w-0 items-center gap-2 text-ink-200">
          <Spinner className="h-3.5 w-3.5" />
          <span className="truncate">
            {t("Scanning")} · {scan.library} {scan.phase === "metadata" ? `· ${t("Metadata")}` : ""}
          </span>
        </span>
        <span className="shrink-0 text-ink-400 tabular-nums">{scan.total ? `${scan.current}/${scan.total}` : scan.current}</span>
      </div>
      <div className="h-1.5 overflow-hidden rounded-full bg-white/10">
        <div className={clsx("h-full rounded-full bg-accent-400 transition-all", !scan.total && "w-1/3 animate-pulse")} style={scan.total ? { width: `${pct}%` } : undefined} />
      </div>
      {scan.message && <div className="truncate text-xs text-ink-500">{scan.message}</div>}
    </div>
  );
}
