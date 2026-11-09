import { useQueryClient } from "@tanstack/react-query";
import clsx from "clsx";
import { Film, Home, LayoutDashboard, LogOut, Search, Settings, Tv } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Link, NavLink, Outlet, useLocation, useNavigate } from "react-router-dom";
import { api } from "../lib/api";
import { isMac } from "../lib/format";
import { useHotkey, useLibraries, useMe, useScrolled } from "../lib/hooks";
import { t } from "../lib/i18n";
import { SearchPalette } from "./Search";
import { Avatar, Logo } from "./ui";

function UserMenu() {
  const { data: me } = useMe();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const qc = useQueryClient();
  const navigate = useNavigate();
  useEffect(() => {
    const close = (e: MouseEvent) => ref.current && !ref.current.contains(e.target as Node) && setOpen(false);
    document.addEventListener("mousedown", close);
    return () => document.removeEventListener("mousedown", close);
  }, []);
  if (!me) return null;
  const logout = async () => {
    await api.post("/api/auth/logout").catch(() => {});
    qc.clear();
    navigate("/login");
  };
  return (
    <div className="relative" ref={ref}>
      <button onClick={() => setOpen((o) => !o)} className="focus-ring rounded-full transition hover:scale-105" aria-label={me.name}>
        <Avatar name={me.name} size={36} className="ring-2 ring-white/10" />
      </button>
      {open && (
        <div className="glass absolute top-12 right-0 w-60 animate-scale-in overflow-hidden rounded-2xl p-1.5 shadow-2xl ring-1 ring-white/10">
          <div className="flex items-center gap-3 px-3 py-3">
            <Avatar name={me.name} size={36} />
            <div className="min-w-0">
              <div className="truncate font-medium">{me.name}</div>
              {me.isAdmin && <div className="text-xs text-accent-400">{t("Administrator")}</div>}
            </div>
          </div>
          <div className="my-1 h-px bg-white/10" />
          {me.isAdmin && (
            <MenuLink to="/admin" icon={<LayoutDashboard size={17} />} onClick={() => setOpen(false)}>
              {t("Dashboard")}
            </MenuLink>
          )}
          <MenuLink to="/settings" icon={<Settings size={17} />} onClick={() => setOpen(false)}>
            {t("Settings")}
          </MenuLink>
          <button onClick={logout} className="flex w-full items-center gap-3 rounded-xl px-3 py-2.5 text-sm text-ink-200 transition hover:bg-white/10 hover:text-white">
            <LogOut size={17} /> {t("Sign out")}
          </button>
        </div>
      )}
    </div>
  );
}

function MenuLink({ to, icon, children, onClick }: { to: string; icon: React.ReactNode; children: React.ReactNode; onClick?: () => void }) {
  return (
    <Link to={to} onClick={onClick} className="flex items-center gap-3 rounded-xl px-3 py-2.5 text-sm text-ink-200 transition hover:bg-white/10 hover:text-white">
      {icon} {children}
    </Link>
  );
}

export function Layout() {
  const scrolled = useScrolled();
  const { data: libs } = useLibraries();
  const [searchOpen, setSearchOpen] = useState(false);
  const loc = useLocation();
  useHotkey("k", (e) => {
    e.preventDefault();
    setSearchOpen(true);
  }, { meta: true });
  useHotkey("/", (e) => {
    const tag = (e.target as HTMLElement)?.tagName;
    if (tag === "INPUT" || tag === "TEXTAREA") return;
    e.preventDefault();
    setSearchOpen(true);
  });
  useEffect(() => {
    window.scrollTo({ top: 0 });
  }, [loc.pathname]);

  const navCls = ({ isActive }: { isActive: boolean }) =>
    clsx("relative rounded-full px-3.5 py-2 text-[14px] font-medium transition", isActive ? "text-white" : "text-ink-300 hover:text-white");

  return (
    <div className="min-h-dvh pb-20 sm:pb-0">
      <header
        className={clsx(
          "fixed inset-x-0 top-0 z-50 transition-all duration-500",
          scrolled ? "glass border-b border-white/[0.06] shadow-lg shadow-black/20" : "bg-gradient-to-b from-black/70 via-black/25 to-transparent",
        )}
      >
        <div className="flex h-16 items-center gap-3 px-5 sm:px-10 lg:px-14">
          <Link to="/" className="focus-ring mr-3 rounded-lg">
            <Logo />
          </Link>
          <nav className="no-scrollbar hidden items-center gap-0.5 overflow-x-auto sm:flex">
            <NavLink to="/" end className={navCls}>
              {t("Home")}
            </NavLink>
            {libs?.map((l) => (
              <NavLink key={l.id} to={`/library/${l.id}`} className={navCls}>
                {l.name}
              </NavLink>
            ))}
          </nav>
          <div className="flex-1" />
          <button
            onClick={() => setSearchOpen(true)}
            className="focus-ring flex h-10 items-center gap-2.5 rounded-full bg-white/[0.08] px-3.5 text-sm text-ink-300 ring-1 ring-white/10 transition hover:bg-white/[0.14] hover:text-white sm:w-56 lg:w-72"
          >
            <Search size={17} />
            <span className="hidden flex-1 text-left sm:block">{t("Search")}</span>
            <kbd className="hidden rounded-md bg-white/10 px-1.5 py-0.5 text-[11px] text-ink-400 lg:block">{isMac ? "⌘K" : "Ctrl K"}</kbd>
          </button>
          <UserMenu />
        </div>
      </header>

      <main key={loc.pathname} className="animate-fade-in">
        <Outlet />
      </main>

      {/* Mobile tab bar */}
      <nav className="glass fixed inset-x-0 bottom-0 z-50 flex border-t border-white/[0.06] pb-[env(safe-area-inset-bottom)] sm:hidden">
        <TabLink to="/" icon={<Home size={21} />} label={t("Home")} end />
        {libs?.slice(0, 3).map((l) => (
          <TabLink key={l.id} to={`/library/${l.id}`} icon={l.type === "tvshows" ? <Tv size={21} /> : <Film size={21} />} label={l.name} />
        ))}
        <button onClick={() => setSearchOpen(true)} className="flex flex-1 flex-col items-center gap-1 py-2.5 text-[11px] text-ink-400">
          <Search size={21} />
          {t("Search")}
        </button>
      </nav>

      <SearchPalette open={searchOpen} onClose={() => setSearchOpen(false)} />
    </div>
  );
}

function TabLink({ to, icon, label, end }: { to: string; icon: React.ReactNode; label: string; end?: boolean }) {
  return (
    <NavLink
      to={to}
      end={end}
      className={({ isActive }) => clsx("flex flex-1 flex-col items-center gap-1 truncate py-2.5 text-[11px]", isActive ? "text-white" : "text-ink-400")}
    >
      {icon}
      <span className="max-w-full truncate px-1">{label}</span>
    </NavLink>
  );
}
