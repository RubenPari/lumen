import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Button, Input, Select, Spinner, Toggle, useToast } from "../../components/ui";
import { api } from "../../lib/api";
import { t } from "../../lib/i18n";
import { LANGS } from "../Auth";
import { Card } from "./ui";

export function AdminSettings() {
  const qc = useQueryClient();
  const toast = useToast();
  const { data } = useQuery({ queryKey: ["settings"], queryFn: () => api.get<Record<string, string>>("/api/admin/settings") });
  const [form, setForm] = useState<Record<string, string> | null>(null);
  const f = form ?? data;
  if (!f)
    return (
      <div className="grid place-items-center py-24">
        <Spinner />
      </div>
    );
  const set = (k: string, v: string) => setForm({ ...f, [k]: v });
  const save = async () => {
    try {
      await api.put("/api/admin/settings", f);
      qc.invalidateQueries({ queryKey: ["settings"] });
      qc.invalidateQueries({ queryKey: ["system"] });
      toast(t("Saved"), "success");
    } catch (e) {
      toast((e as Error).message, "error");
    }
  };
  return (
    <div className="max-w-2xl space-y-4">
      <h2 className="font-display text-xl font-semibold">{t("Settings")}</h2>
      <Card className="space-y-5">
        <Input label={t("Server name")} value={f.server_name} onChange={(e) => set("server_name", e.target.value)} />
        <Toggle checked={f.hide_users === "true"} onChange={(v) => set("hide_users", String(v))} label={t("Hide user list on login")} />
      </Card>
      <Card className="space-y-5">
        <h3 className="font-display font-semibold">{t("Metadata")}</h3>
        <Input
          label={t("TMDB API key")}
          value={f.tmdb_api_key}
          onChange={(e) => set("tmdb_api_key", e.target.value)}
          hint={t("Optional — enables posters, backdrops, cast and plot summaries.")}
          className="font-mono text-[13px]"
        />
        <Select label={t("Metadata language")} value={f.metadata_language} onChange={(v) => set("metadata_language", v)} options={LANGS.map(([v, l]) => ({ value: v, label: l }))} />
        <Input label={t("Scan interval (hours)")} value={f.scan_interval_hours} onChange={(e) => set("scan_interval_hours", e.target.value)} inputMode="numeric" />
      </Card>
      <Card className="space-y-5">
        <h3 className="font-display font-semibold">{t("Transcoding")}</h3>
        <Select
          label={t("Hardware acceleration")}
          value={f.hw_accel}
          onChange={(v) => set("hw_accel", v)}
          options={[
            { value: "", label: t("None") },
            { value: "vaapi", label: "VA-API (Intel/AMD, Linux)" },
            { value: "qsv", label: "Intel Quick Sync" },
            { value: "nvenc", label: "NVIDIA NVENC" },
            { value: "videotoolbox", label: "Apple VideoToolbox" },
          ]}
        />
      </Card>
      <div className="flex justify-end">
        <Button variant="primary" onClick={save}>
          {t("Save")}
        </Button>
      </div>
    </div>
  );
}
