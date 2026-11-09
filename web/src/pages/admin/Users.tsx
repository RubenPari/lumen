import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Clock, Pencil, Plus, Shield, Trash2 } from "lucide-react";
import { useState } from "react";
import { Avatar, Button, Input, Modal, Toggle, useToast } from "../../components/ui";
import { api, type User } from "../../lib/api";
import { useMe } from "../../lib/hooks";
import { t } from "../../lib/i18n";
import { Card } from "./ui";

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
