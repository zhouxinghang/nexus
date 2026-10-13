import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { CheckCircle2, XCircle, Info, X } from "lucide-react";
import { cn } from "@/lib/api";
import { Button } from "@/components/ui";

/*
 * A dependency-free toast system in the house style: a context provider that
 * mirrors ThemeProvider/AuthProvider, an imperative `useToast()` API, and a
 * single fixed-corner `Toaster` viewport. No external toast library — a small
 * local component in the spirit of the portal-free `Modal`.
 */

export type ToastVariant = "success" | "error" | "info";

type ToastOptions = {
  description?: string;
  variant?: ToastVariant;
  // Milliseconds before auto-dismiss. 0 keeps the toast until dismissed.
  duration?: number;
};

type Toast = {
  id: number;
  title: string;
  description?: string;
  variant: ToastVariant;
  duration: number;
};

type ToastInput = { title: string } & ToastOptions;

type ToastApi = {
  (input: ToastInput): number;
  success: (message: string, opts?: ToastOptions) => number;
  error: (message: string, opts?: ToastOptions) => number;
  info: (message: string, opts?: ToastOptions) => number;
  dismiss: (id: number) => void;
};

// Per-variant defaults: errors linger long enough to read; success/info are
// brief acknowledgements.
const DEFAULT_DURATION: Record<ToastVariant, number> = {
  success: 3000,
  info: 3000,
  error: 6000,
};

const ToastContext = createContext<ToastApi | null>(null);

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const nextId = useRef(1);
  // Track per-toast dismiss timers so manual dismiss / unmount can clear them.
  const timers = useRef<Map<number, ReturnType<typeof setTimeout>>>(new Map());

  const dismiss = useCallback((id: number) => {
    setToasts((prev) => prev.filter((t) => t.id !== id));
    const handle = timers.current.get(id);
    if (handle) {
      clearTimeout(handle);
      timers.current.delete(id);
    }
  }, []);

  const push = useCallback(
    (input: ToastInput): number => {
      const id = nextId.current++;
      const variant = input.variant ?? "info";
      const duration =
        input.duration ?? DEFAULT_DURATION[variant];
      const toast: Toast = {
        id,
        title: input.title,
        description: input.description,
        variant,
        duration,
      };
      setToasts((prev) => [...prev, toast]);
      if (duration > 0) {
        const handle = setTimeout(() => dismiss(id), duration);
        timers.current.set(id, handle);
      }
      return id;
    },
    [dismiss],
  );

  // Clear any outstanding timers on unmount.
  useEffect(() => {
    const map = timers.current;
    return () => {
      map.forEach((h) => clearTimeout(h));
      map.clear();
    };
  }, []);

  const api = useMemo<ToastApi>(() => {
    const fn = ((input: ToastInput) => push(input)) as ToastApi;
    fn.success = (message, opts) =>
      push({ title: message, ...opts, variant: "success" });
    fn.error = (message, opts) =>
      push({ title: message, ...opts, variant: "error" });
    fn.info = (message, opts) =>
      push({ title: message, ...opts, variant: "info" });
    fn.dismiss = dismiss;
    return fn;
  }, [push, dismiss]);

  return (
    <ToastContext.Provider value={api}>
      {children}
      <Toaster toasts={toasts} onDismiss={dismiss} />
    </ToastContext.Provider>
  );
}

export function useToast() {
  const ctx = useContext(ToastContext);
  if (!ctx) throw new Error("useToast must be used within ToastProvider");
  return ctx;
}

/* ---------------------------------------------------------------- Toaster */

const toastVariants = cva(
  "pointer-events-auto flex w-full items-start gap-3 rounded-lg border bg-card p-3.5 pr-2 text-card-foreground shadow-pop animate-toast-in",
  {
    variants: {
      variant: {
        success: "border-success/30",
        error: "border-destructive/30",
        info: "border-border",
      },
    },
    defaultVariants: { variant: "info" },
  },
);

const iconFor = {
  success: CheckCircle2,
  error: XCircle,
  info: Info,
} as const;

const iconTone = {
  success: "text-success",
  error: "text-destructive",
  info: "text-muted-foreground",
} as const;

function Toaster({
  toasts,
  onDismiss,
}: {
  toasts: Toast[];
  onDismiss: (id: number) => void;
}) {
  if (toasts.length === 0) return null;

  return (
    <div
      // Polite by default; error toasts carry their own role="alert" so screen
      // readers interrupt for failures but not for routine confirmations.
      aria-live="polite"
      className="pointer-events-none fixed inset-x-0 bottom-0 z-[60] flex flex-col items-center gap-2 p-4 sm:inset-x-auto sm:bottom-4 sm:right-4 sm:max-w-sm sm:items-end"
    >
      {toasts.map((t) => {
        const Icon = iconFor[t.variant];
        return (
          <div
            key={t.id}
            role={t.variant === "error" ? "alert" : "status"}
            className={cn(toastVariants({ variant: t.variant }), "sm:w-96")}
          >
            <Icon size={18} className={cn("mt-0.5 shrink-0", iconTone[t.variant])} />
            <div className="min-w-0 flex-1 pt-0.5">
              <p className="text-sm font-medium leading-snug">{t.title}</p>
              {t.description && (
                <p className="mt-0.5 text-sm text-muted-foreground">
                  {t.description}
                </p>
              )}
            </div>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label="Dismiss notification"
              onClick={() => onDismiss(t.id)}
              className="shrink-0"
            >
              <X size={15} />
            </Button>
          </div>
        );
      })}
    </div>
  );
}

export type { ToastOptions, ToastApi };
export type ToastVariantProps = VariantProps<typeof toastVariants>;
