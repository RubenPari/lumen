import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import clsx from "clsx";
import { Activity, Cpu, Film, HardDrive, Play, RefreshCw, Tv } from "lucide-react";
import { LandscapeCard } from "../../components/cards";
import { Avatar, Button, Spinner, useToast } from "../../components/ui";
import { api, type Item, type ScanStatus } from "../../lib/api";
import { formatBytes, formatClock } from "../../lib/format";
import { t } from "../../lib/i18n";
import { Card, ScanBar, Stat } from "./ui";

interface Dashboard {
  counts: Record<string, number>;
  totalSize: number;
  totalHours: number;
  playing: { userId: string; userName: string; itemId: string; itemName: string; seriesName?: string; position: number; duration: number; paused: boolean; method: string; device: string }[];
  transcodes: number;
  scan: ScanStatus;
  version: string;
  uptime: number;
  ffmpeg: string;
  ffprobe: string;
  goVersion: string;
  memoryMB: number;
  recent: Item[];
  users: number;
}

export function AdminDashboard() {
  const qc = useQueryClient();
  const toast = useToast();
  const { data } = useQuery({ queryKey: ["dashboard"], queryFn: () => api.get<Dashboard>("/api/admin/dashboard"), refetchInterval: 3000 });
  const scan = useMutation({
    mutationFn: () => api.post("/api/scan"),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["dashboard"] }),
    onError: (e) => toast((e as Error).message, "error"),
  });
  if (!data)
    return (
      <div className="grid place-items-center py-24">
        <Spinner />
      </div>
    );
  const uptime = data.uptime > 86400 ? `${Math.floor(data.uptime / 86400)}d` : data.uptime > 3600 ? `${Math.floor(data.uptime / 3600)}h` : `${Math.floor(data.uptime / 60)}m`;
  return (
    <div className="space-y-6">
      <div className="grid grid-cols-2 gap-3 sm:gap-4 xl:grid-cols-4">
        <Stat icon={<Film size={22} />} label={t("Movies")} value={data.counts.Movie ?? 0} />
        <Stat icon={<Tv size={22} />} label={t("Shows")} value={data.counts.Series ?? 0} />
        <Stat icon={<Play size={22} />} label={t("Episodes")} value={data.counts.Episode ?? 0} />
        <Stat icon={<HardDrive size={22} />} label={t("Storage")} value={formatBytes(data.totalSize)} />
      </div>
      <div className="grid gap-6 xl:grid-cols-3">
        <Card className="xl:col-span-2">
          <div className="mb-4 flex items-center justify-between">
            <h2 className="flex items-center gap-2 font-display text-lg font-semibold">
              <Activity size={18} className="text-accent-400" /> {t("Now playing")}
            </h2>
            <span className="text-[13px] text-ink-400">
              {t("Active streams")}: {data.transcodes}
            </span>
          </div>
          {data.playing.length === 0 ? (
            <p className="py-8 text-center text-ink-400">{t("Nobody is watching right now")}</p>
          ) : (
            <div className="space-y-3">
              {data.playing.map((p) => (
                <div key={p.userId} className="flex items-center gap-4 rounded-2xl bg-white/[0.04] p-3">
                  <Avatar name={p.userName} size={40} />
                  <div className="min-w-0 flex-1">
                    <div className="truncate font-medium">
                      {p.seriesName ? `${p.seriesName} · ` : ""}
                      {p.itemName}
                    </div>
                    <div className="truncate text-[13px] text-ink-400">
                      {p.userName} · {p.device} · {p.method === "Transcode" ? t("Transcoding") : t("Direct play")}
                      {p.paused ? " · ⏸" : ""}
                    </div>
                    <div className="mt-2 h-1 overflow-hidden rounded-full bg-white/10">
                      <div className="h-full bg-accent-400" style={{ width: `${p.duration ? (p.position / p.duration) * 100 : 0}%` }} />
                    </div>
                  </div>
                  <div className="shrink-0 text-xs text-ink-400 tabular-nums">
                    {formatClock(p.position)} / {formatClock(p.duration)}
                  </div>
                </div>
              ))}
            </div>
          )}
        </Card>
        <Card className="space-y-4">
          <h2 className="flex items-center gap-2 font-display text-lg font-semibold">
            <Cpu size={18} className="text-accent-400" /> {t("Server")}
          </h2>
          <dl className="space-y-2 text-[14px]">
            {[
              ["Version", data.version],
              [t("Uptime"), uptime],
              [t("Total runtime"), `${Math.round(data.totalHours)} h`],
              [t("Users"), data.users],
              ["Go", data.goVersion],
              ["RAM", `${data.memoryMB} MB`],
              ["ffmpeg", data.ffmpeg || t("not found")],
              ["ffprobe", data.ffprobe || t("not found")],
            ].map(([k, v]) => (
              <div key={String(k)} className="flex justify-between gap-4">
                <dt className="text-ink-400">{k}</dt>
                <dd className={clsx("truncate text-right font-mono text-[12px]", !v || v === t("not found") ? "text-amber-300" : "text-ink-200")}>{v}</dd>
              </div>
            ))}
          </dl>
          <ScanBar scan={data.scan} />
          {data.scan.lastError && <div className="rounded-xl bg-red-500/10 p-3 text-xs text-red-300">{data.scan.lastError}</div>}
          <Button className="w-full" onClick={() => scan.mutate()} disabled={data.scan.running}>
            <RefreshCw size={16} className={data.scan.running ? "animate-spin" : ""} /> {t("Scan all libraries")}
          </Button>
        </Card>
      </div>
      {data.recent.length > 0 && (
        <Card>
          <h2 className="mb-4 font-display text-lg font-semibold">{t("Recently added")}</h2>
          <div className="grid grid-cols-2 gap-4 md:grid-cols-3 2xl:grid-cols-4">
            {data.recent.map((it) => (
              <LandscapeCard key={it.id} item={it} />
            ))}
          </div>
        </Card>
      )}
    </div>
  );
}
