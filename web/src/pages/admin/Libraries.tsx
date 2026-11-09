import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Film, FolderOpen, Pencil, Plus, RefreshCw, Trash2, Tv } from "lucide-react";
import { useState } from "react";
import { LibraryForm, type LibraryDraft } from "../../components/LibraryForm";
import { Button, EmptyState, Modal, useToast } from "../../components/ui";
import { api, type Library, type ScanStatus } from "../../lib/api";
import { formatRelative } from "../../lib/format";
import { t } from "../../lib/i18n";
import { Card, ScanBar } from "./ui";

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
                    setDraft({ name: l.name, type: l.type, paths: l.paths ?? [], realtime: l.realtime });
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
