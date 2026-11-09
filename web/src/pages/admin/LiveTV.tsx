import { useQuery, useQueryClient } from "@tanstack/react-query";
import clsx from "clsx";
import { Plus, RefreshCw, Trash2, Tv } from "lucide-react";
import { useState } from "react";
import { Button, EmptyState, Input, Modal, Spinner, useToast } from "../../components/ui";
import { api } from "../../lib/api";
import { formatRelative } from "../../lib/format";
import { t } from "../../lib/i18n";
import { Card } from "./ui";

interface LiveSource {
  id: string;
  kind: "m3u" | "xmltv";
  name: string;
  url: string;
  userAgent?: string;
  lastRefresh: number;
  lastError?: string;
  itemCount: number;
}

export function AdminLiveTV() {
  const qc = useQueryClient();
  const toast = useToast();
  const { data: sources } = useQuery({ queryKey: ["livetv-sources"], queryFn: () => api.get<LiveSource[]>("/api/admin/livetv/sources") });
  const [form, setForm] = useState<{ kind: "m3u" | "xmltv"; name: string; url: string; userAgent: string } | null>(null);
  const [busy, setBusy] = useState(false);
  const refreshAll = () => {
    qc.invalidateQueries({ queryKey: ["livetv-sources"] });
    qc.invalidateQueries({ queryKey: ["livetv"] });
    qc.invalidateQueries({ queryKey: ["system"] });
  };
  const add = async () => {
    if (!form) return;
    setBusy(true);
    try {
      const s = await api.post<LiveSource>("/api/admin/livetv/sources", form);
      if (s.lastError) toast(s.lastError, "error");
      else toast(t("Saved"), "success");
      setForm(null);
      refreshAll();
    } catch (e) {
      toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  };
  const refresh = async () => {
    setBusy(true);
    try {
      await api.post("/api/admin/livetv/refresh");
      refreshAll();
    } finally {
      setBusy(false);
    }
  };
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="font-display text-xl font-semibold">{t("Live TV")}</h2>
        <div className="flex gap-2">
          <Button onClick={refresh} disabled={busy || !sources?.length}>
            <RefreshCw size={16} className={busy ? "animate-spin" : ""} /> {t("Refresh")}
          </Button>
          <Button variant="primary" onClick={() => setForm({ kind: "m3u", name: "", url: "", userAgent: "" })}>
            <Plus size={17} /> {t("Add source")}
          </Button>
        </div>
      </div>
      <p className="text-sm text-ink-400">{t("Add an M3U playlist with your channels and, optionally, an XMLTV guide. Channels are matched to the guide by tvg-id or name.")}</p>
      {sources?.length === 0 && <EmptyState icon={<Tv size={28} />} title={t("No channels")} />}
      <div className="grid gap-4 md:grid-cols-2">
        {sources?.map((s) => (
          <Card key={s.id} className="space-y-3">
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <div className="mb-1 inline-flex rounded-md bg-white/10 px-2 py-0.5 text-[11px] font-bold tracking-wider text-ink-300 uppercase">{s.kind === "m3u" ? "M3U" : "XMLTV"}</div>
                <div className="truncate font-display text-lg font-semibold">{s.name}</div>
                <div className="truncate font-mono text-[12px] text-ink-400">{s.url}</div>
              </div>
              <Button
                size="sm"
                variant="danger"
                onClick={async () => {
                  await api.del(`/api/admin/livetv/sources/${s.id}`);
                  refreshAll();
                }}
              >
                <Trash2 size={14} />
              </Button>
            </div>
            <div className="flex flex-wrap gap-x-4 text-[13px] text-ink-400">
              <span>{s.kind === "m3u" ? t("{0} channels", s.itemCount) : t("{0} programs", s.itemCount)}</span>
              <span>
                {t("Last refresh")}: {formatRelative(s.lastRefresh)}
              </span>
            </div>
            {s.lastError && <div className="rounded-xl bg-red-500/10 p-3 text-xs text-red-300">{s.lastError}</div>}
          </Card>
        ))}
      </div>
      <Modal open={!!form} onClose={() => setForm(null)} title={t("Add source")}>
        {form && (
          <div className="space-y-4">
            <div className="grid grid-cols-2 gap-3">
              {(["m3u", "xmltv"] as const).map((k) => (
                <button
                  key={k}
                  onClick={() => setForm({ ...form, kind: k })}
                  className={clsx("rounded-2xl p-4 text-left ring-1 transition ring-inset", form.kind === k ? "bg-accent-500/15 ring-accent-400" : "bg-white/[0.04] ring-white/10")}
                >
                  <div className="font-semibold">{k === "m3u" ? t("M3U playlist") : t("XMLTV guide")}</div>
                  <div className="text-xs text-ink-400">{k === "m3u" ? t("Channels") : t("Guide")}</div>
                </button>
              ))}
            </div>
            <Input label={t("Name")} value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
            <Input label={t("URL or file path")} value={form.url} onChange={(e) => setForm({ ...form, url: e.target.value })} className="font-mono text-[13px]" placeholder="https://… /path/list.m3u" />
            {form.kind === "m3u" && <Input label="User-Agent" value={form.userAgent} onChange={(e) => setForm({ ...form, userAgent: e.target.value })} hint={t("Optional")} />}
          </div>
        )}
        <div className="mt-6 flex justify-end gap-2">
          <Button variant="ghost" onClick={() => setForm(null)}>
            {t("Cancel")}
          </Button>
          <Button variant="primary" onClick={add} disabled={busy || !form?.url.trim()}>
            {busy && <Spinner className="h-4 w-4" />} {t("Add")}
          </Button>
        </div>
      </Modal>
    </div>
  );
}
