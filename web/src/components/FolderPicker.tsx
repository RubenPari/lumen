import { useQuery } from "@tanstack/react-query";
import { ChevronLeft, Folder, HardDrive } from "lucide-react";
import { useState } from "react";
import { api } from "../lib/api";
import { t } from "../lib/i18n";
import { Button, Input, Modal, Spinner } from "./ui";

interface FSResult {
  path: string;
  parent: string;
  dirs: { name: string; path: string }[];
}

export function FolderPicker({ open, onClose, onPick }: { open: boolean; onClose: () => void; onPick: (path: string) => void }) {
  const [path, setPath] = useState("");
  const [manual, setManual] = useState("");
  const { data, isLoading, error } = useQuery({
    queryKey: ["fs", path],
    queryFn: () => api.get<FSResult>(`/api/admin/fs?path=${encodeURIComponent(path)}`),
    enabled: open,
  });
  return (
    <Modal open={open} onClose={onClose} title={t("Choose folder")}>
      <div className="mb-3 flex items-center gap-2">
        <Button size="icon" variant="secondary" disabled={!path} onClick={() => setPath(data?.parent ?? "")} aria-label={t("Back")}>
          <ChevronLeft size={18} />
        </Button>
        <div className="min-w-0 flex-1 truncate rounded-xl bg-white/5 px-3 py-2.5 font-mono text-[13px] text-ink-200">{path || "—"}</div>
      </div>
      <div className="h-80 overflow-y-auto rounded-2xl bg-black/20 p-1.5 ring-1 ring-white/5">
        {isLoading && (
          <div className="grid h-full place-items-center">
            <Spinner />
          </div>
        )}
        {error && <div className="p-4 text-sm text-red-300">{(error as Error).message}</div>}
        {data?.dirs.map((d) => (
          <button
            key={d.path}
            onClick={() => setPath(d.path)}
            className="flex w-full items-center gap-3 rounded-xl px-3 py-2.5 text-left text-[14px] transition hover:bg-white/10"
          >
            {path ? <Folder size={18} className="shrink-0 text-accent-400" /> : <HardDrive size={18} className="shrink-0 text-ink-300" />}
            <span className="truncate">{d.name}</span>
          </button>
        ))}
        {data && data.dirs.length === 0 && <div className="p-4 text-sm text-ink-400">—</div>}
      </div>
      <div className="mt-4 flex gap-2">
        <Input placeholder="/path/to/media" value={manual} onChange={(e) => setManual(e.target.value)} className="h-10 font-mono text-[13px]" />
        <Button disabled={!manual.startsWith("/")} onClick={() => setPath(manual)}>
          →
        </Button>
      </div>
      <div className="mt-5 flex justify-end gap-2">
        <Button variant="ghost" onClick={onClose}>
          {t("Cancel")}
        </Button>
        <Button
          variant="primary"
          disabled={!path}
          onClick={() => {
            onPick(path);
            onClose();
          }}
        >
          {t("Select this folder")}
        </Button>
      </div>
    </Modal>
  );
}
