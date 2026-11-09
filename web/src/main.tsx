import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { lazy, StrictMode, Suspense, type ReactNode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter, Navigate, Route, Routes, useLocation } from "react-router-dom";
import { Layout } from "./components/Layout";
import { Button, EmptyState, Spinner, ToastProvider } from "./components/ui";
import "./index.css";
import { ApiError } from "./lib/api";
import { AudioProvider } from "./lib/audio";
import { useMe, useSystemInfo } from "./lib/hooks";
import { t, currentLang } from "./lib/i18n";
import { Login, Setup } from "./pages/Auth";
import { Home } from "./pages/Home";
import { ItemPage } from "./pages/Item";
import { Browse, Library } from "./pages/Library";
import { UserSettings } from "./pages/UserSettings";

document.documentElement.lang = currentLang();

const Reader = lazy(() => import("./pages/Reader").then((m) => ({ default: m.Reader })));
const Player = lazy(() => import("./pages/Player").then((m) => ({ default: m.Player })));
const AdminLayout = lazy(() => import("./pages/admin/Layout").then((m) => ({ default: m.AdminLayout })));
const AdminDashboard = lazy(() => import("./pages/admin/Dashboard").then((m) => ({ default: m.AdminDashboard })));
const AdminLibraries = lazy(() => import("./pages/admin/Libraries").then((m) => ({ default: m.AdminLibraries })));
const AdminUsers = lazy(() => import("./pages/admin/Users").then((m) => ({ default: m.AdminUsers })));
const AdminLiveTV = lazy(() => import("./pages/admin/LiveTV").then((m) => ({ default: m.AdminLiveTV })));
const LiveTV = lazy(() => import("./pages/LiveTV").then((m) => ({ default: m.LiveTV })));
const LivePlayer = lazy(() => import("./pages/LivePlayer").then((m) => ({ default: m.LivePlayer })));
const AdminSettings = lazy(() => import("./pages/admin/Settings").then((m) => ({ default: m.AdminSettings })));

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 15_000,
      refetchOnWindowFocus: false,
      retry: (n, e) => !(e instanceof ApiError && e.status < 500) && n < 2,
    },
  },
});

function Loading() {
  return (
    <div className="grid h-dvh place-items-center">
      <Spinner className="h-8 w-8" />
    </div>
  );
}

/** Routes requiring authentication; redirects to setup/login when needed. */
function Guard({ children, admin }: { children: ReactNode; admin?: boolean }) {
  const sys = useSystemInfo();
  const me = useMe();
  const loc = useLocation();
  if (sys.isLoading || me.isLoading) return <Loading />;
  if (sys.data?.setupRequired) return <Navigate to="/setup" replace />;
  if (!me.data) return <Navigate to="/login" replace state={{ from: loc.pathname }} />;
  if (admin && !me.data.isAdmin) return <Navigate to="/" replace />;
  return <>{children}</>;
}

function PublicOnly({ children, setup }: { children: ReactNode; setup?: boolean }) {
  const sys = useSystemInfo();
  const me = useMe();
  if (sys.isLoading || me.isLoading) return <Loading />;
  if (setup && !sys.data?.setupRequired) return <Navigate to="/" replace />;
  if (!setup && sys.data?.setupRequired) return <Navigate to="/setup" replace />;
  if (!setup && me.data) return <Navigate to="/" replace />;
  return <>{children}</>;
}

function NotFound() {
  return (
    <div className="pt-24">
      <EmptyState title={t("Not found")}>
        <a href="/">
          <Button variant="primary" className="mt-4">
            {t("Go home")}
          </Button>
        </a>
      </EmptyState>
    </div>
  );
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <ToastProvider>
        <AudioProvider>
        <BrowserRouter>
          <Suspense fallback={<Loading />}>
          <Routes>
            <Route path="/setup" element={<PublicOnly setup><Setup /></PublicOnly>} />
            <Route path="/login" element={<PublicOnly><Login /></PublicOnly>} />
            <Route path="/play/:id" element={<Guard><Player /></Guard>} />
            <Route path="/read/:id" element={<Guard><Reader /></Guard>} />
            <Route path="/live/:id" element={<Guard><LivePlayer /></Guard>} />
            <Route element={<Guard><Layout /></Guard>}>
              <Route index element={<Home />} />
              <Route path="library/:id" element={<Library />} />
              <Route path="browse" element={<Browse />} />
              <Route path="item/:id" element={<ItemPage />} />
              <Route path="settings" element={<UserSettings />} />
              <Route path="livetv" element={<LiveTV />} />
              <Route path="admin" element={<Guard admin><AdminLayout /></Guard>}>
                <Route index element={<AdminDashboard />} />
                <Route path="libraries" element={<AdminLibraries />} />
                <Route path="users" element={<AdminUsers />} />
                <Route path="settings" element={<AdminSettings />} />
                <Route path="livetv" element={<AdminLiveTV />} />
              </Route>
              <Route path="*" element={<NotFound />} />
            </Route>
          </Routes>
          </Suspense>
        </BrowserRouter>
        </AudioProvider>
      </ToastProvider>
    </QueryClientProvider>
  </StrictMode>,
);
