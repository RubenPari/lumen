import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import clsx from "clsx";
import { Activity, Clock, Cpu, Film, FolderOpen, HardDrive, LayoutDashboard, Library as LibIcon, Pencil, Play, Plus, RefreshCw, Settings, Shield, Trash2, Tv, Users } from "lucide-react";
import { useState } from "react";
import { NavLink, Outlet } from "react-router-dom";
import { LandscapeCard } from "../components/cards";
import { LibraryForm, type LibraryDraft } from "../components/LibraryForm";
import { Avatar, Button, EmptyState, Input, Modal, Select, Spinner, Toggle, useToast } from "../components/ui";
import { api, type Item, type Library, type ScanStatus, type User } from "../lib/api";
import { formatBytes, formatClock, formatRelative } from "../lib/format";
import { useMe } from "../lib/hooks";
import { t } from "../lib/i18n";
import { LANGS } from "./Auth";

export function AdminLayout() {
  const link = ({ isActive }: { isActive: boolean }) =>
    clsx("flex shrink-0 items-center gap-3 rounded-xl px-3.5 py-2.5 text-[14px] font-medium transition", isActive ? "bg-white/10 text-white" : "text-ink-300 hover:bg-white/5 hover:text-white");
  return (
    <div className="px-5 pt-24 pb-16 sm:px-10 lg:px-14">
      <h1 className="mb-6 font-display text-3xl font-bold tracking-tight">{t("Administration")}</h1>
      <div className="flex flex-col gap-8 lg:flex-row">
        <nav className="no-scrollbar -mx-5 flex gap-1 overflow-x-auto px-5 lg:mx-0 lg:w-56 lg:flex-col lg:px-0">
          <NavLink to="/admin" end className={link}>
            <LayoutDashboard size={18} /> {t("Dashboard")}
          </NavLink>
          <NavLink to="/admin/libraries" className={link}>
            <LibIcon size={18} /> {t("Libraries")}
          </NavLink>
          <NavLink to="/admin/users" className={link}>
            <Users size={18} /> {t("Users")}
          </NavLink>
          <NavLink to="/admin/settings" className={link}>
            <Settings size={18} /> {t("Settings")}
          </NavLink>
        </nav>
        <div className="min-w-0 flex-1">
          <Outlet />
        </div>
      </div>
    </div>
  );
}

function Card({ children, className }: { children: React.ReactNode; className?: string }) {
  return <div className={clsx("rounded-3xl bg-white/[0.035] p-5 ring-1 ring-white/[0.07] sm:p-6", className)}>{children}</div>;
}

function Stat({ icon, label, value }: { icon: React.ReactNode; label: string; value: React.ReactNode }) {
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

export function AdminLibraries() {
  const qc = useQueryClient();
  const toast = useToast();
  const { data: libs } = useQuery({ queryKey: ["libraries"], queryFn: () => api.get<Library[]>("/api/libraries") });
  const { data: scan } = useQuery({ queryKey: ["scan"], queryFn: () => api.get<ScanStatus>("/api/scan/status"), refetchInterval: 2000 });
  const [editing, setEditing] = useState<Library | null>(null);
  const [creating, setCreating] = useState(false);
  const [draft, setDraft] = useState<LibraryDraft>({ name: "", type: "movies", paths: [] });
  const [deleting, setDeleting] = useState<Library | null>(null);
  const refresh = () => {
    qc.invalidateQueries({ queryKey: ["libraries"] });
    qc.invalidateQueries({ queryKey: ["home"] });
    qc.invalidateQueries({ queryKey: ["scan"] });
  };
  const save = async () => {
    try {
      if (editing) await api.patch(`/api/libraries/${editing.id}`, draft);
      else await api.post("/api/libraries", draft);
      setCreating(false);
      setEditing(null);
      refresh();
      toast(t("Saved"), "success");
    } catch (e) {
      toast((e as Error).message, "error");
    }
  };
  const scanLib = async (id: string) => {
    try {
      await api.post(`/api/libraries/${id}/scan`);
      refresh();
    } catch (e) {
      toast((e as Error).message, "error");
    }
  };
  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="font-display text-xl font-semibold">{t("Libraries")}</h2>
        <Button
          variant="primary"
          onClick={() => {
            setDraft({ name: t("Movies"), type: "movies", paths: [] });
            setCreating(true);
          }}
        >
          <Plus size={17} /> {t("Add library")}
        </Button>
      </div>
      {scan?.running && (
        <Card>
          <ScanBar scan={scan} />
        </Card>
      )}
      {libs?.length === 0 && <EmptyState icon={<FolderOpen size={28} />} title={t("No libraries yet")}>{t("Add a library to start building your collection.")}</EmptyState>}
      <div className="grid gap-4 md:grid-cols-2">
        {libs?.map((l) => (
          <Card key={l.id} className="relative overflow-hidden !p-0">
            <div className="relative grid h-32 grid-cols-4 overflow-hidden">
              {(l.images.length ? l.images : [null]).slice(0, 4).map((id, i) =>
                id ? (
                  <img key={i} src={`/api/items/${id}/images/backdrop?w=400`} className="h-full w-full object-cover" alt="" />
                ) : (
                  <div key={i} className="col-span-4 bg-gradient-to-br from-ink-700 to-ink-850" />
                ),
              )}
              <div className="absolute inset-0 bg-gradient-to-t from-ink-900 via-ink-900/60 to-transparent" />
              <div className="absolute bottom-3 left-5 flex items-center gap-2.5">
                {l.type === "movies" ? <Film size={20} className="text-accent-400" /> : <Tv size={20} className="text-accent-400" />}
                <span className="font-display text-xl font-semibold">{l.name}</span>
              </div>
            </div>
            <div className="space-y-3 p-5">
              <div className="flex flex-wrap gap-x-4 gap-y-1 text-[13px] text-ink-400">
                <span>{t("{0} items", l.itemCount)}</span>
                <span>
                  {t("Last scan")}: {formatRelative(l.lastScan)}
                </span>
              </div>
              <div className="space-y-1">
                {l.paths?.map((p) => (
                  <div key={p} className="truncate font-mono text-[12px] text-ink-300">
                    {p}
                  </div>
                ))}
              </div>
              <div className="flex gap-2 pt-1">
                <Button size="sm" onClick={() => scanLib(l.id)} disabled={scan?.running}>
                  <RefreshCw size={14} /> {t("Scan")}
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => {
                    setDraft({ name: l.name, type: l.type, paths: l.paths ?? [] });
                    setEditing(l);
                  }}
                >
                  <Pencil size={14} /> {t("Edit")}
                </Button>
                <div className="flex-1" />
                <Button size="sm" variant="danger" onClick={() => setDeleting(l)}>
                  <Trash2 size={14} />
                </Button>
              </div>
            </div>
          </Card>
        ))}
      </div>
      <Modal
        open={creating || !!editing}
        onClose={() => {
          setCreating(false);
          setEditing(null);
        }}
        title={editing ? t("Edit library") : t("Add library")}
      >
        <LibraryForm value={draft} onChange={setDraft} lockType={!!editing} />
        <div className="mt-6 flex justify-end gap-2">
          <Button
            variant="ghost"
            onClick={() => {
              setCreating(false);
              setEditing(null);
            }}
          >
            {t("Cancel")}
          </Button>
          <Button variant="primary" onClick={save} disabled={!draft.name || !draft.paths.length}>
            {t("Save")}
          </Button>
        </div>
      </Modal>
      <Modal open={!!deleting} onClose={() => setDeleting(null)} title={t("Delete library?")}>
        <p className="mb-6 text-ink-300">
          <strong className="text-white">{deleting?.name}</strong> — {t("Files on disk are not touched.")}
        </p>
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={() => setDeleting(null)}>
            {t("Cancel")}
          </Button>
          <Button
            variant="danger"
            onClick={async () => {
              await api.del(`/api/libraries/${deleting!.id}`);
              setDeleting(null);
              refresh();
            }}
          >
            <Trash2 size={15} /> {t("Delete")}
          </Button>
        </div>
      </Modal>
    </div>
  );
}

export function AdminUsers() {
  const qc = useQueryClient();
  const toast = useToast();
  const { data: me } = useMe();
  const { data: users } = useQuery({ queryKey: ["users"], queryFn: () => api.get<User[]>("/api/admin/users") });
  const [edit, setEdit] = useState<User | "new" | null>(null);
  const [form, setForm] = useState({ name: "", password: "", isAdmin: false });
  const open = (u: User | "new") => {
    setEdit(u);
    setForm(u === "new" ? { name: "", password: "", isAdmin: false } : { name: u.name, password: "", isAdmin: u.isAdmin });
  };
  const save = async () => {
    try {
      if (edit === "new") await api.post("/api/admin/users", form);
      else if (edit) await api.patch(`/api/admin/users/${edit.id}`, { name: form.name, isAdmin: form.isAdmin, ...(form.password ? { password: form.password } : {}) });
      setEdit(null);
      qc.invalidateQueries({ queryKey: ["users"] });
      qc.invalidateQueries({ queryKey: ["public-users"] });
      toast(t("Saved"), "success");
    } catch (e) {
      toast((e as Error).message, "error");
    }
  };
  const remove = async (u: User) => {
    try {
      await api.del(`/api/admin/users/${u.id}`);
      qc.invalidateQueries({ queryKey: ["users"] });
    } catch (e) {
      toast((e as Error).message, "error");
    }
  };
  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="font-display text-xl font-semibold">{t("Users")}</h2>
        <Button variant="primary" onClick={() => open("new")}>
          <Plus size={17} /> {t("New user")}
        </Button>
      </div>
      <Card className="divide-y divide-white/[0.06] !p-2">
        {users?.map((u) => (
          <div key={u.id} className="flex items-center gap-4 px-3 py-3">
            <Avatar name={u.name} size={42} />
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2 font-medium">
                {u.name}
                {u.isAdmin && (
                  <span className="inline-flex items-center gap-1 rounded-full bg-accent-500/15 px-2 py-0.5 text-[11px] text-accent-300">
                    <Shield size={11} /> Admin
                  </span>
                )}
              </div>
              <div className="text-[13px] text-ink-400">
                <Clock size={12} className="mr-1 inline" />
                {new Date(u.createdAt * 1000).toLocaleDateString()}
              </div>
            </div>
            <Button size="sm" variant="ghost" onClick={() => open(u)}>
              <Pencil size={14} />
            </Button>
            {u.id !== me?.id && (
              <Button size="sm" variant="danger" onClick={() => remove(u)}>
                <Trash2 size={14} />
              </Button>
            )}
          </div>
        ))}
      </Card>
      <Modal open={!!edit} onClose={() => setEdit(null)} title={edit === "new" ? t("New user") : t("Edit")}>
        <div className="space-y-4">
          <Input label={t("Name")} value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
          <Input
            type="password"
            label={edit === "new" ? t("Password") : t("New password")}
            value={form.password}
            onChange={(e) => setForm({ ...form, password: e.target.value })}
            hint={edit === "new" ? t("Leave empty for no password") : undefined}
          />
          <Toggle checked={form.isAdmin} onChange={(v) => setForm({ ...form, isAdmin: v })} label={t("Administrator")} />
        </div>
        <div className="mt-6 flex justify-end gap-2">
          <Button variant="ghost" onClick={() => setEdit(null)}>
            {t("Cancel")}
          </Button>
          <Button variant="primary" onClick={save} disabled={!form.name.trim()}>
            {t("Save")}
          </Button>
        </div>
      </Modal>
    </div>
  );
}

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
