import clsx from "clsx";
import { Film, FolderPlus, Trash2, Tv } from "lucide-react";
import { useState } from "react";
import { t } from "../lib/i18n";
import { FolderPicker } from "./FolderPicker";
import { Button, Input } from "./ui";

export interface LibraryDraft {
  name: string;
  type: "movies" | "tvshows";
  paths: string[];
}

export function LibraryForm({ value, onChange, lockType }: { value: LibraryDraft; onChange: (v: LibraryDraft) => void; lockType?: boolean }) {
  const [picker, setPicker] = useState(false);
  return (
    <div className="space-y-5">
      {!lockType && (
        <div>
          <div className="mb-2 text-[13px] font-medium text-ink-300">{t("Content type")}</div>
          <div className="grid grid-cols-2 gap-3">
            {(
              [
                ["movies", t("Movies"), Film],
                ["tvshows", t("Shows"), Tv],
              ] as const
            ).map(([type, label, Icon]) => (
              <button
                key={type}
                onClick={() => onChange({ ...value, type, name: value.name || label })}
                className={clsx(
                  "flex items-center gap-3 rounded-2xl p-4 text-left ring-1 transition ring-inset",
                  value.type === type ? "bg-accent-500/15 ring-accent-400" : "bg-white/[0.04] ring-white/10 hover:bg-white/[0.08]",
                )}
              >
                <Icon size={22} className={value.type === type ? "text-accent-400" : "text-ink-300"} />
                <span className="font-medium">{label}</span>
              </button>
            ))}
          </div>
        </div>
      )}
      <Input label={t("Name")} value={value.name} onChange={(e) => onChange({ ...value, name: e.target.value })} />
      <div>
        <div className="mb-2 text-[13px] font-medium text-ink-300">{t("Folders")}</div>
        <div className="space-y-2">
          {value.paths.map((p) => (
            <div key={p} className="flex items-center gap-2 rounded-xl bg-white/[0.05] py-1.5 pr-1.5 pl-4 ring-1 ring-white/10">
              <span className="min-w-0 flex-1 truncate font-mono text-[13px]">{p}</span>
              <button
                onClick={() => onChange({ ...value, paths: value.paths.filter((x) => x !== p) })}
                className="grid h-8 w-8 place-items-center rounded-lg text-ink-400 hover:bg-white/10 hover:text-red-300"
              >
                <Trash2 size={16} />
              </button>
            </div>
          ))}
          <Button variant="secondary" className="w-full" onClick={() => setPicker(true)}>
            <FolderPlus size={17} /> {t("Add folder")}
          </Button>
        </div>
      </div>
      <FolderPicker
        open={picker}
        onClose={() => setPicker(false)}
        onPick={(p) => !value.paths.includes(p) && onChange({ ...value, paths: [...value.paths, p] })}
      />
    </div>
  );
}
