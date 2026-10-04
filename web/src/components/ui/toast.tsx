"use client";

import { Toast as T } from "radix-ui";
import { CircleAlert, CircleCheck, X } from "lucide-react";
import { createContext, useCallback, useContext, useState, type ReactNode } from "react";

type ToastInput = { title: string; description?: string; tone?: "success" | "danger" };
type ToastItem = ToastInput & { id: number };

const ToastContext = createContext<(toast: ToastInput) => void>(() => {});

export function useToast() {
  return useContext(ToastContext);
}

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<ToastItem[]>([]);
  const push = useCallback((toast: ToastInput) => {
    setToasts((current) => [...current, { ...toast, id: Date.now() + Math.random() }]);
  }, []);

  return (
    <ToastContext.Provider value={push}>
      <T.Provider swipeDirection="right" duration={4000}>
        {children}
        {toasts.map((toast) => {
          const Icon = toast.tone === "danger" ? CircleAlert : CircleCheck;
          return (
            <T.Root
              key={toast.id}
              onOpenChange={(open) => {
                if (!open) setToasts((current) => current.filter((item) => item.id !== toast.id));
              }}
              className="flex w-full items-start gap-3 rounded-md border border-border-strong bg-surface p-4 shadow-md data-[state=open]:animate-[halo-toast_220ms_var(--ease-brand)] data-[swipe=move]:translate-x-(--radix-toast-swipe-move-x)"
            >
              <Icon
                aria-hidden="true"
                size={20}
                strokeWidth={1.75}
                className={toast.tone === "danger" ? "mt-px shrink-0 text-danger" : "mt-px shrink-0 text-success"}
              />
              <div className="flex min-w-0 flex-1 flex-col gap-1">
                <T.Title className="text-body-sm font-semibold text-fg">{toast.title}</T.Title>
                {toast.description ? <T.Description className="text-body-sm text-fg-3">{toast.description}</T.Description> : null}
              </div>
              <T.Close aria-label="Dismiss" className="grid size-6 place-items-center rounded-sm text-fg-3 hover:bg-hover hover:text-fg">
                <X aria-hidden="true" size={16} strokeWidth={1.75} />
              </T.Close>
            </T.Root>
          );
        })}
        <T.Viewport className="fixed right-0 bottom-0 z-[60] flex w-[min(400px,100vw)] flex-col gap-2 p-6 outline-none" />
      </T.Provider>
    </ToastContext.Provider>
  );
}
