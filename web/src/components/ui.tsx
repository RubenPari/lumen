import clsx from "clsx";
import { X } from "lucide-react";
import { createContext, useCallback, useContext, useEffect, useState, type ButtonHTMLAttributes, type InputHTMLAttributes, type ReactNode } from "react";
import { createPortal } from "react-dom";

type Variant = "primary" | "secondary" | "ghost" | "danger" | "glass";
type Size = "sm" | "md" | "lg" | "icon";

export function Button({
  variant = "secondary",
  size = "md",
  className,
  children,
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: Variant; size?: Size }) {
  return (
    <button
      className={clsx(
        "focus-ring inline-flex shrink-0 items-center justify-center gap-2 rounded-full font-medium whitespace-nowrap transition duration-200 select-none disabled:pointer-events-none disabled:opacity-50 active:scale-[0.97]",
        {
          primary: "bg-white text-ink-950 hover:bg-accent-300 shadow-lg shadow-black/30",
          secondary: "bg-white/10 text-white hover:bg-white/20 ring-1 ring-inset ring-white/10",
          glass: "glass text-white ring-1 ring-inset ring-white/15 hover:bg-white/15",
          ghost: "text-ink-200 hover:bg-white/10 hover:text-white",
          danger: "bg-red-500/15 text-red-300 hover:bg-red-500/25 ring-1 ring-inset ring-red-400/20",
        }[variant],
        {
          sm: "h-8 px-3.5 text-[13px]",
          md: "h-10 px-5 text-sm",
          lg: "h-12 px-7 text-[15px]",
          icon: "h-10 w-10",
        }[size],
        className,
      )}
      {...props}
    >
      {children}
    </button>
  );
}

export function IconButton({
  label,
  className,
  active,
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & { label: string; active?: boolean }) {
  return (
    <button
      aria-label={label}
      title={label}
      className={clsx(
        "focus-ring grid h-10 w-10 shrink-0 place-items-center rounded-full transition duration-200 active:scale-90",
        active ? "bg-white text-ink-950" : "bg-white/10 text-white ring-1 ring-inset ring-white/10 hover:bg-white/20",
        className,
      )}
      {...props}
    />
  );
}

export function Input({ className, label, hint, ...props }: InputHTMLAttributes<HTMLInputElement> & { label?: string; hint?: string }) {
  const input = (
    <input
      className={clsx(
        "h-11 w-full rounded-xl bg-white/[0.06] px-4 text-[15px] text-white ring-1 ring-inset ring-white/10 transition placeholder:text-ink-400 hover:ring-white/20 focus:bg-white/[0.08] focus:ring-2 focus:ring-accent-400 focus:outline-none",
        className,
      )}
      {...props}
    />
  );
  if (!label) return input;
  return (
    <label className="block space-y-1.5">
      <span className="text-[13px] font-medium text-ink-300">{label}</span>
      {input}
      {hint && <span className="block text-xs text-ink-400">{hint}</span>}
    </label>
  );
}

export function Select({
  value,
  onChange,
  options,
  className,
  label,
}: {
  value: string;
  onChange: (v: string) => void;
  options: { value: string; label: string }[];
  className?: string;
  label?: string;
}) {
  const sel = (
    <select
      value={value}
      onChange={(e) => onChange(e.target.value)}
      className={clsx(
        "focus-ring h-10 cursor-pointer appearance-none rounded-full bg-white/[0.07] bg-[length:14px] bg-[right_14px_center] bg-no-repeat pr-10 pl-4 text-sm text-white ring-1 ring-inset ring-white/10 transition hover:bg-white/[0.12]",
        className,
      )}
      style={{
        backgroundImage:
          "url(\"data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' fill='none' stroke='%23a3a8b6' stroke-width='2.5'%3E%3Cpath d='m6 9 6 6 6-6'/%3E%3C/svg%3E\")",
      }}
    >
      {options.map((o) => (
        <option key={o.value} value={o.value} className="bg-ink-800">
          {o.label}
        </option>
      ))}
    </select>
  );
  if (!label) return sel;
  return (
    <label className="block space-y-1.5">
      <span className="block text-[13px] font-medium text-ink-300">{label}</span>
      {sel}
    </label>
  );
}

export function Toggle({ checked, onChange, label, description }: { checked: boolean; onChange: (v: boolean) => void; label: string; description?: string }) {
  return (
    <button type="button" onClick={() => onChange(!checked)} className="focus-ring flex w-full items-center justify-between gap-4 rounded-xl py-1 text-left">
      <span>
        <span className="block text-[15px] text-white">{label}</span>
        {description && <span className="block text-[13px] text-ink-400">{description}</span>}
      </span>
      <span className={clsx("relative h-6 w-11 shrink-0 rounded-full transition", checked ? "bg-accent-500" : "bg-white/15")}>
        <span className={clsx("absolute top-0.5 h-5 w-5 rounded-full bg-white shadow transition-all", checked ? "left-[22px]" : "left-0.5")} />
      </span>
    </button>
  );
}

export function Chip({ active, children, onClick }: { active?: boolean; children: ReactNode; onClick?: () => void }) {
  return (
    <button
      onClick={onClick}
      className={clsx(
        "focus-ring h-9 shrink-0 rounded-full px-4 text-[13px] font-medium transition",
        active ? "bg-white text-ink-950" : "bg-white/[0.07] text-ink-200 ring-1 ring-inset ring-white/10 hover:bg-white/[0.14] hover:text-white",
      )}
    >
      {children}
    </button>
  );
}

export function Spinner({ className }: { className?: string }) {
  return <span className={clsx("inline-block h-5 w-5 animate-spin rounded-full border-2 border-white/20 border-t-white", className)} />;
}

export function Modal({
  open,
  onClose,
  title,
  children,
  wide,
}: {
  open: boolean;
  onClose: () => void;
  title?: ReactNode;
  children: ReactNode;
  wide?: boolean;
}) {
  useEffect(() => {
    if (!open) return;
    const k = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", k);
    const prev = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      window.removeEventListener("keydown", k);
      document.body.style.overflow = prev;
    };
  }, [open, onClose]);
  if (!open) return null;
  return createPortal(
    <div className="fixed inset-0 z-[100] flex items-end justify-center p-0 sm:items-center sm:p-6">
      <div className="absolute inset-0 animate-fade-in bg-black/70 backdrop-blur-sm" onClick={onClose} />
      <div
        role="dialog"
        className={clsx(
          "relative max-h-[92dvh] w-full animate-scale-in overflow-y-auto rounded-t-3xl bg-ink-850 shadow-2xl ring-1 ring-white/10 sm:rounded-3xl",
          wide ? "sm:max-w-3xl" : "sm:max-w-lg",
        )}
      >
        {title && (
          <div className="sticky top-0 z-10 flex items-center justify-between gap-4 bg-ink-850/95 px-6 pt-6 pb-4 backdrop-blur">
            <h2 className="font-display text-xl font-semibold">{title}</h2>
            <button onClick={onClose} className="focus-ring grid h-9 w-9 place-items-center rounded-full text-ink-300 hover:bg-white/10 hover:text-white">
              <X size={18} />
            </button>
          </div>
        )}
        <div className={clsx("px-6 pb-6", !title && "pt-6")}>{children}</div>
      </div>
    </div>,
    document.body,
  );
}

// ---- Toasts ----

interface Toast {
  id: number;
  text: string;
  tone: "info" | "error" | "success";
}

const ToastCtx = createContext<(text: string, tone?: Toast["tone"]) => void>(() => {});

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const push = useCallback((text: string, tone: Toast["tone"] = "info") => {
    const id = Date.now() + Math.random();
    setToasts((t) => [...t, { id, text, tone }]);
    setTimeout(() => setToasts((t) => t.filter((x) => x.id !== id)), 3800);
  }, []);
  return (
    <ToastCtx.Provider value={push}>
      {children}
      {createPortal(
        <div className="pointer-events-none fixed inset-x-0 bottom-24 z-[200] flex flex-col items-center gap-2 px-4 sm:bottom-8">
          {toasts.map((t) => (
            <div
              key={t.id}
              className={clsx(
                "glass pointer-events-auto animate-fade-up rounded-full px-5 py-3 text-sm font-medium shadow-2xl ring-1",
                t.tone === "error" ? "text-red-200 ring-red-400/30" : t.tone === "success" ? "text-emerald-200 ring-emerald-400/30" : "text-white ring-white/15",
              )}
            >
              {t.text}
            </div>
          ))}
        </div>,
        document.body,
      )}
    </ToastCtx.Provider>
  );
}

export const useToast = () => useContext(ToastCtx);

export function EmptyState({ icon, title, children }: { icon?: ReactNode; title: string; children?: ReactNode }) {
  return (
    <div className="flex flex-col items-center justify-center gap-3 py-24 text-center">
      {icon && <div className="grid h-16 w-16 place-items-center rounded-2xl bg-white/5 text-ink-300 ring-1 ring-white/10">{icon}</div>}
      <h3 className="font-display text-xl font-semibold">{title}</h3>
      {children && <div className="max-w-md text-ink-400">{children}</div>}
    </div>
  );
}

export function Logo({ className }: { className?: string }) {
  return (
    <span className={clsx("inline-flex items-center gap-2.5 font-display text-xl font-semibold tracking-tight", className)}>
      <svg viewBox="0 0 64 64" className="h-8 w-8" aria-hidden>
        <defs>
          <linearGradient id="lg" x1="0" y1="0" x2="1" y2="1">
            <stop offset="0" stopColor="#ffd27a" />
            <stop offset="1" stopColor="#f0743e" />
          </linearGradient>
        </defs>
        <rect width="64" height="64" rx="18" fill="url(#lg)" opacity="0.14" />
        <path d="M24 18v28l22-14z" fill="url(#lg)" />
      </svg>
      <span>Lumen</span>
    </span>
  );
}

export function Avatar({ name, size = 40, className }: { name: string; size?: number; className?: string }) {
  const hue = [...name].reduce((a, c) => a + c.charCodeAt(0), 0) % 360;
  return (
    <span
      className={clsx("inline-grid shrink-0 place-items-center rounded-full font-display font-semibold text-white uppercase", className)}
      style={{
        width: size,
        height: size,
        fontSize: size * 0.4,
        background: `linear-gradient(135deg, hsl(${hue} 70% 55%), hsl(${(hue + 40) % 360} 75% 40%))`,
      }}
    >
      {name.slice(0, 1)}
    </span>
  );
}
