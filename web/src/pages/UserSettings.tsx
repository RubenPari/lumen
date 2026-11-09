import { useState } from "react";
import { Button, Input, Select, Toggle, useToast } from "../components/ui";
import { api } from "../lib/api";
import { usePrefs } from "../lib/hooks";
import { currentLang, setLang, t } from "../lib/i18n";

const LANG_OPTIONS = [
  { value: "", label: "—" },
  { value: "en", label: "English" },
  { value: "it", label: "Italiano" },
  { value: "fr", label: "Français" },
  { value: "de", label: "Deutsch" },
  { value: "es", label: "Español" },
  { value: "ja", label: "日本語" },
];

const ISO2: Record<string, string> = { en: "eng", it: "ita", fr: "fre", de: "ger", es: "spa", ja: "jpn" };

export function UserSettings() {
  const [prefs, setPrefs] = usePrefs();
  const toast = useToast();
  const [cur, setCur] = useState("");
  const [nw, setNw] = useState("");
  const change = async () => {
    try {
      await api.post("/api/me/password", { current: cur, new: nw });
      setCur("");
      setNw("");
      toast(t("Saved"), "success");
    } catch (e) {
      toast((e as Error).message, "error");
    }
  };
  const fromIso = (v: string) => Object.entries(ISO2).find(([, x]) => x === v)?.[0] ?? v;
  return (
    <div className="mx-auto max-w-2xl space-y-6 px-5 pt-24 pb-16">
      <h1 className="font-display text-3xl font-bold tracking-tight">{t("Settings")}</h1>
      <section className="space-y-5 rounded-3xl bg-white/[0.035] p-6 ring-1 ring-white/[0.07]">
        <h2 className="font-display text-lg font-semibold">{t("Player")}</h2>
        <Select
          label="Interface language / Lingua interfaccia"
          value={currentLang() === "it" ? "it" : "en"}
          onChange={setLang}
          options={[
            { value: "en", label: "English" },
            { value: "it", label: "Italiano" },
          ]}
        />
        <Select
          label={t("Preferred subtitle language")}
          value={fromIso(prefs.subtitleLang)}
          onChange={(v) => setPrefs({ subtitleLang: ISO2[v] ?? v })}
          options={LANG_OPTIONS}
        />
        <Select label={t("Preferred audio language")} value={fromIso(prefs.audioLang)} onChange={(v) => setPrefs({ audioLang: ISO2[v] ?? v })} options={LANG_OPTIONS} />
        <Select
          label={t("Subtitle size")}
          value={prefs.subtitleSize}
          onChange={(v) => setPrefs({ subtitleSize: v as "small" | "medium" | "large" })}
          options={[
            { value: "small", label: t("Small") },
            { value: "medium", label: t("Medium") },
            { value: "large", label: t("Large") },
          ]}
        />
        <Toggle checked={prefs.autoplayNext} onChange={(v) => setPrefs({ autoplayNext: v })} label={t("Autoplay next episode")} />
      </section>
      <section className="space-y-4 rounded-3xl bg-white/[0.035] p-6 ring-1 ring-white/[0.07]">
        <h2 className="font-display text-lg font-semibold">{t("Change password")}</h2>
        <Input type="password" label={t("Current password")} value={cur} onChange={(e) => setCur(e.target.value)} autoComplete="current-password" />
        <Input type="password" label={t("New password")} value={nw} onChange={(e) => setNw(e.target.value)} autoComplete="new-password" />
        <div className="flex justify-end">
          <Button variant="primary" onClick={change}>
            {t("Save")}
          </Button>
        </div>
      </section>
    </div>
  );
}
