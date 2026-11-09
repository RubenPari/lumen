import clsx from "clsx";
import { LayoutDashboard, Library as LibIcon, Radio, Settings, Users } from "lucide-react";
import { NavLink, Outlet } from "react-router-dom";
import { t } from "../../lib/i18n";

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
          <NavLink to="/admin/livetv" className={link}>
            <Radio size={18} /> {t("Live TV")}
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
