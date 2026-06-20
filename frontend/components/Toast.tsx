"use client";

import { createContext, useContext, useState, useCallback, useRef, useEffect } from "react";
import { CheckCircle2, XCircle, Info, X } from "lucide-react";
import { cn } from "@/lib/cn";

// ── Types ──────────────────────────────────────────────────────────────────────

export type ToastType = "success" | "error" | "info";

interface ToastItem {
  id: string;
  type: ToastType;
  message: string;
}

interface ToastContextValue {
  toast: (message: string, type?: ToastType) => void;
}

// ── Context ────────────────────────────────────────────────────────────────────

const ToastContext = createContext<ToastContextValue | null>(null);

export function useToast(): ToastContextValue {
  const ctx = useContext(ToastContext);
  if (!ctx) throw new Error("useToast must be used within ToastProvider");
  return ctx;
}

// ── Single Toast ───────────────────────────────────────────────────────────────

const DURATIONS: Record<ToastType, number> = {
  success: 3000,
  error: 4000,
  info: 2500,
};

function ToastItem({
  item,
  onDismiss,
}: {
  item: ToastItem;
  onDismiss: (id: string) => void;
}) {
  const [visible, setVisible] = useState(false);
  const [shake, setShake] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    // Mount → animate in
    const raf = requestAnimationFrame(() => setVisible(true));
    // Error shake
    if (item.type === "error") {
      const t = setTimeout(() => setShake(true), 20);
      const t2 = setTimeout(() => setShake(false), 350);
      return () => { clearTimeout(t); clearTimeout(t2); cancelAnimationFrame(raf); };
    }
    return () => cancelAnimationFrame(raf);
  }, [item.type]);

  useEffect(() => {
    timer.current = setTimeout(() => {
      setVisible(false);
      setTimeout(() => onDismiss(item.id), 300);
    }, DURATIONS[item.type]);
    return () => { if (timer.current) clearTimeout(timer.current); };
  }, [item.id, item.type, onDismiss]);

  const icon = {
    success: <CheckCircle2 size={16} className="flex-shrink-0" style={{ color: "var(--accent)" }} />,
    error:   <XCircle      size={16} className="flex-shrink-0" style={{ color: "var(--error)" }} />,
    info:    <Info         size={16} className="flex-shrink-0" style={{ color: "var(--signal)" }} />,
  }[item.type];

  const borderColor = {
    success: "rgba(0,229,160,0.25)",
    error:   "rgba(255,64,88,0.25)",
    info:    "rgba(77,168,255,0.25)",
  }[item.type];

  const bg = {
    success: "rgba(0,229,160,0.06)",
    error:   "rgba(255,64,88,0.06)",
    info:    "rgba(77,168,255,0.06)",
  }[item.type];

  return (
    <div
      role="alert"
      aria-live="assertive"
      aria-atomic="true"
      className={cn(
        "flex items-center gap-3 px-4 py-3 rounded-2xl shadow-xl max-w-[360px] w-full",
        "transition-all duration-300",
        visible ? "opacity-100 translate-y-0" : "opacity-0 translate-y-4",
        shake ? "shake" : "",
      )}
      style={{
        background: `color-mix(in srgb, var(--surface-2) 94%, transparent)`,
        backdropFilter: "blur(16px)",
        WebkitBackdropFilter: "blur(16px)",
        border: `1px solid ${borderColor}`,
        boxShadow: "0 8px 24px rgba(0,0,0,0.7), 0 2px 8px rgba(0,0,0,0.5)",
      }}
    >
      {/* Accent stripe */}
      <div
        className="absolute left-0 top-3 bottom-3 w-[3px] rounded-full"
        style={{ background: borderColor }}
        aria-hidden="true"
      />

      <div className="pl-1 flex items-center gap-3 flex-1 min-w-0">
        {icon}
        <span
          className="text-sm font-medium leading-snug truncate"
          style={{ color: "var(--text-1)" }}
        >
          {item.message}
        </span>
      </div>

      <button
        onClick={() => { setVisible(false); setTimeout(() => onDismiss(item.id), 300); }}
        aria-label="Bildirimi kapat"
        className="flex-shrink-0 w-6 h-6 rounded-full flex items-center justify-center transition-colors duration-100 hover:bg-white/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]"
        style={{ color: "var(--text-3)" }}
      >
        <X size={12} />
      </button>
    </div>
  );
}

// ── Provider ───────────────────────────────────────────────────────────────────

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [toasts, setToasts] = useState<ToastItem[]>([]);
  const counter = useRef(0);

  const dismiss = useCallback((id: string) => {
    setToasts((prev) => prev.filter((t) => t.id !== id));
  }, []);

  const toast = useCallback((message: string, type: ToastType = "info") => {
    const id = `toast-${Date.now()}-${++counter.current}`;
    setToasts((prev) => [...prev.slice(-3), { id, type, message }]);
  }, []);

  return (
    <ToastContext.Provider value={{ toast }}>
      {children}

      {/* Toast container — bottom center, above GravityWell */}
      <div
        aria-label="Bildirimler"
        className="fixed bottom-0 left-0 right-0 z-[60] flex flex-col items-center gap-2 px-4 pointer-events-none"
        style={{
          paddingBottom: "calc(104px + env(safe-area-inset-bottom, 0px))",
        }}
      >
        {toasts.map((item) => (
          <div key={item.id} className="pointer-events-auto w-full flex justify-center">
            <ToastItem item={item} onDismiss={dismiss} />
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}
