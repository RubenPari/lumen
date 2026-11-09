import { useQuery, useQueryClient } from "@tanstack/react-query";
import clsx from "clsx";
import { ArrowLeft, ArrowRight, Check, Film, Lock, Sparkles, Tv } from "lucide-react";
import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { LibraryForm, type LibraryDraft } from "../components/LibraryForm";
import { Avatar, Button, Input, Logo, Select, Spinner } from "../components/ui";
import { api, ApiError, type User } from "../lib/api";
import { useSystemInfo } from "../lib/hooks";
import { t } from "../lib/i18n";

function Aurora() {
  return (
    <div className="pointer-events-none fixed inset-0 overflow-hidden">
      <div className="absolute -top-1/3 -left-1/4 h-[80vmax] w-[80vmax] rounded-full bg-[radial-gradient(circle,rgba(248,169,58,0.20),transparent_60%)] blur-3xl" />
      <div className="absolute -right-1/4 -bottom-1/3 h-[80vmax] w-[80vmax] rounded-full bg-[radial-gradient(circle,rgba(120,80,255,0.16),transparent_60%)] blur-3xl" />
      <div className="absolute inset-0 bg-[url('data:image/svg+xml;utf8,<svg xmlns=%22http://www.w3.org/2000/svg%22 width=%22140%22 height=%22140%22><filter id=%22n%22><feTurbulence baseFrequency=%220.9%22/></filter><rect width=%22100%25%22 height=%22100%25%22 filter=%22url(%23n)%22 opacity=%220.035%22/></svg>')]" />
    </div>
  );
}

export function Login() {
  const qc = useQueryClient();
  const navigate = useNavigate();
  const { data: sys } = useSystemInfo();
  const { data: users } = useQuery({
    queryKey: ["public-users"],
    queryFn: () => api.get<{ id: string; name: string; hasPassword: boolean }[]>("/api/users/public"),
  });
  const [selected, setSelected] = useState<string | null>(null);
  const [manual, setManual] = useState(false);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  const login = async (name: string, pw: string) => {
    setBusy(true);
    setErr("");
    try {
      const r = await api.post<{ user: User }>("/api/auth/login", { username: name, password: pw });
      qc.setQueryData(["me"], r.user);
      await qc.invalidateQueries();
      navigate("/", { replace: true });
    } catch (e) {
      setErr(e instanceof ApiError && e.status === 401 ? t("Invalid username or password") : (e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const showPicker = !manual && !selected && users && users.length > 0;
  const sel = users?.find((u) => u.id === selected);

  return (
    <div className="relative grid min-h-dvh place-items-center px-5 py-16">
      <Aurora />
      <div className="relative w-full max-w-3xl animate-fade-up text-center">
        <Logo className="mb-10 text-2xl" />
        {showPicker ? (
          <>
            <h1 className="mb-10 font-display text-3xl font-semibold sm:text-4xl">{t("Who's watching?")}</h1>
            <div className="mb-10 flex flex-wrap justify-center gap-6 sm:gap-10">
              {users!.map((u) => (
                <button
                  key={u.id}
                  onClick={() => (u.hasPassword ? setSelected(u.id) : login(u.name, ""))}
                  className="group focus-ring flex w-28 flex-col items-center gap-3 rounded-3xl"
                >
                  <Avatar name={u.name} size={104} className="rounded-3xl text-4xl shadow-xl ring-2 ring-transparent transition duration-300 group-hover:scale-105 group-hover:ring-white/70" />
                  <span className="max-w-full truncate text-ink-300 transition group-hover:text-white">{u.name}</span>
                </button>
              ))}
            </div>
            <Button variant="ghost" onClick={() => setManual(true)}>
              {t("Use another account")}
            </Button>
          </>
        ) : (
          <form
            onSubmit={(e) => {
              e.preventDefault();
              login(sel ? sel.name : username, password);
            }}
            className="glass mx-auto max-w-sm space-y-4 rounded-3xl p-7 text-left shadow-2xl ring-1 ring-white/10"
          >
            {sel ? (
              <div className="mb-2 flex flex-col items-center gap-3">
                <Avatar name={sel.name} size={72} className="rounded-2xl" />
                <div className="font-display text-xl font-semibold">{sel.name}</div>
              </div>
            ) : (
              <>
                <h1 className="mb-1 font-display text-2xl font-semibold">{t("Sign in")}</h1>
                <p className="mb-2 text-sm text-ink-400">{sys?.serverName}</p>
                <Input autoFocus label={t("Username")} value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username" />
              </>
            )}
            <Input
              autoFocus={!!sel}
              type="password"
              label={t("Password")}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="current-password"
            />
            {err && <div className="rounded-xl bg-red-500/10 px-4 py-2.5 text-sm text-red-300 ring-1 ring-red-400/20">{err}</div>}
            <Button type="submit" variant="primary" className="w-full" size="lg" disabled={busy}>
              {busy ? <Spinner className="h-4 w-4 border-ink-950/30 border-t-ink-950" /> : <Lock size={17} />} {t("Sign in")}
            </Button>
            {users && users.length > 0 && (
              <Button
                type="button"
                variant="ghost"
                className="w-full"
                onClick={() => {
                  setSelected(null);
                  setManual(false);
                  setPassword("");
                  setErr("");
                }}
              >
                <ArrowLeft size={16} /> {t("Back")}
              </Button>
            )}
          </form>
        )}
      </div>
    </div>
  );
}

const LANGS = [
  ["en-US", "English"],
  ["it-IT", "Italiano"],
  ["fr-FR", "Français"],
  ["de-DE", "Deutsch"],
  ["es-ES", "Español"],
  ["pt-BR", "Português (BR)"],
  ["ja-JP", "日本語"],
];

export function Setup() {
  const qc = useQueryClient();
  const navigate = useNavigate();
  const [step, setStep] = useState(0);
  const [serverName, setServerName] = useState("Lumen");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [password2, setPassword2] = useState("");
  const [libs, setLibs] = useState<LibraryDraft[]>([]);
  const [draft, setDraft] = useState<LibraryDraft>({ name: t("Movies"), type: "movies", paths: [] });
  const [tmdb, setTmdb] = useState("");
  const [lang, setLang] = useState(navigator.language.startsWith("it") ? "it-IT" : "en-US");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");

  const steps = [t("Welcome to Lumen"), t("Create your account"), t("Add your media"), t("Metadata"), t("All set!")];

  const createAccount = async () => {
    setBusy(true);
    setErr("");
    try {
      const r = await api.post<{ user: User }>("/api/setup", { username, password, serverName });
      qc.setQueryData(["me"], r.user);
      qc.invalidateQueries({ queryKey: ["system"] });
      setStep(2);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const finish = async () => {
    setBusy(true);
    setErr("");
    try {
      await api.put("/api/admin/settings", { tmdb_api_key: tmdb, metadata_language: lang, server_name: serverName });
      for (const l of libs) await api.post("/api/libraries", l);
      setStep(4);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="relative grid min-h-dvh place-items-center px-5 py-12">
      <Aurora />
      <div className="relative w-full max-w-xl animate-fade-up">
        <div className="mb-8 flex items-center justify-between">
          <Logo />
          <div className="flex gap-1.5">
            {steps.map((_, i) => (
              <span key={i} className={clsx("h-1.5 rounded-full transition-all duration-500", i === step ? "w-8 bg-accent-400" : i < step ? "w-3 bg-accent-400/60" : "w-3 bg-white/15")} />
            ))}
          </div>
        </div>
        <div className="glass rounded-3xl p-7 shadow-2xl ring-1 ring-white/10 sm:p-9">
          <h1 className="mb-2 font-display text-3xl font-semibold tracking-tight">{steps[step]}</h1>

          {step === 0 && (
            <div className="space-y-6">
              <p className="text-ink-300">{t("Your movies and shows, beautifully organized.")}</p>
              <div className="grid grid-cols-3 gap-3 py-2">
                {[Film, Tv, Sparkles].map((Icon, i) => (
                  <div key={i} className="grid aspect-square place-items-center rounded-2xl bg-white/[0.04] ring-1 ring-white/10">
                    <Icon size={30} className="text-accent-400" strokeWidth={1.5} />
                  </div>
                ))}
              </div>
              <Input label={t("Server name")} value={serverName} onChange={(e) => setServerName(e.target.value)} />
              <Button variant="primary" size="lg" className="w-full" onClick={() => setStep(1)}>
                {t("Get started")} <ArrowRight size={18} />
              </Button>
            </div>
          )}

          {step === 1 && (
            <form
              className="space-y-4"
              onSubmit={(e) => {
                e.preventDefault();
                createAccount();
              }}
            >
              <p className="mb-4 text-ink-300">{t("This will be the administrator account.")}</p>
              <Input autoFocus label={t("Username")} value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username" />
              <Input type="password" label={t("Password")} value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="new-password" />
              <Input type="password" label={t("Password") + " (2)"} value={password2} onChange={(e) => setPassword2(e.target.value)} autoComplete="new-password" />
              {err && <div className="rounded-xl bg-red-500/10 px-4 py-2.5 text-sm text-red-300">{err}</div>}
              <div className="flex gap-2 pt-2">
                <Button type="button" variant="ghost" onClick={() => setStep(0)}>
                  <ArrowLeft size={17} /> {t("Back")}
                </Button>
                <Button type="submit" variant="primary" className="flex-1" disabled={!username.trim() || password !== password2 || busy}>
                  {busy && <Spinner className="h-4 w-4" />} {t("Continue")} <ArrowRight size={17} />
                </Button>
              </div>
            </form>
          )}

          {step === 2 && (
            <div className="space-y-5">
              <p className="text-ink-300">{t("You can add libraries later from the dashboard.")}</p>
              {libs.map((l, i) => (
                <div key={i} className="flex items-center gap-3 rounded-2xl bg-white/[0.05] p-4 ring-1 ring-white/10">
                  {l.type === "movies" ? <Film size={20} className="text-accent-400" /> : <Tv size={20} className="text-accent-400" />}
                  <div className="min-w-0 flex-1">
                    <div className="font-medium">{l.name}</div>
                    <div className="truncate font-mono text-xs text-ink-400">{l.paths.join(", ")}</div>
                  </div>
                  <button onClick={() => setLibs(libs.filter((_, j) => j !== i))} className="text-ink-400 hover:text-red-300">
                    ×
                  </button>
                </div>
              ))}
              <div className="rounded-2xl bg-black/20 p-5 ring-1 ring-white/5">
                <LibraryForm value={draft} onChange={setDraft} />
                <Button
                  className="mt-4 w-full"
                  disabled={!draft.name || draft.paths.length === 0}
                  onClick={() => {
                    setLibs([...libs, draft]);
                    setDraft({ name: draft.type === "movies" ? t("Shows") : t("Movies"), type: draft.type === "movies" ? "tvshows" : "movies", paths: [] });
                  }}
                >
                  <Check size={17} /> {t("Add library")}
                </Button>
              </div>
              <div className="flex justify-end">
                <Button variant="primary" onClick={() => setStep(3)}>
                  {libs.length ? t("Continue") : t("Skip")} <ArrowRight size={17} />
                </Button>
              </div>
            </div>
          )}

          {step === 3 && (
            <div className="space-y-5">
              <Input
                label={t("TMDB API key")}
                value={tmdb}
                onChange={(e) => setTmdb(e.target.value)}
                hint={t("Optional — enables posters, backdrops, cast and plot summaries.")}
                className="font-mono text-[13px]"
              />
              <Select label={t("Metadata language")} value={lang} onChange={setLang} options={LANGS.map(([v, l]) => ({ value: v, label: l }))} className="w-full" />
              {err && <div className="rounded-xl bg-red-500/10 px-4 py-2.5 text-sm text-red-300">{err}</div>}
              <div className="flex gap-2 pt-2">
                <Button variant="ghost" onClick={() => setStep(2)}>
                  <ArrowLeft size={17} /> {t("Back")}
                </Button>
                <Button variant="primary" className="flex-1" onClick={finish} disabled={busy}>
                  {busy && <Spinner className="h-4 w-4" />} {t("Finish")} <Check size={17} />
                </Button>
              </div>
            </div>
          )}

          {step === 4 && (
            <div className="space-y-6 text-center">
              <div className="mx-auto grid h-20 w-20 place-items-center rounded-full bg-accent-500/15 text-accent-400 ring-1 ring-accent-400/30">
                <Check size={38} strokeWidth={2.5} />
              </div>
              <Button
                variant="primary"
                size="lg"
                className="w-full"
                onClick={async () => {
                  await qc.invalidateQueries();
                  navigate("/", { replace: true });
                }}
              >
                {t("Start watching")} <ArrowRight size={18} />
              </Button>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

export { LANGS };
