"use client";

import { Dialog as D, VisuallyHidden } from "radix-ui";
import { X } from "lucide-react";
import type { ReactNode } from "react";
import { cn } from "@/lib/cn";

export function Dialog({
  open,
  onOpenChange,
  title,
  description,
  width = "default",
  children,
  footer,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: ReactNode;
  width?: "default" | "wide";
  children?: ReactNode;
  footer?: ReactNode;
}) {
  return (
    <D.Root open={open} onOpenChange={onOpenChange}>
      <D.Portal>
        <D.Overlay className="fixed inset-0 z-40 bg-ink/80 data-[state=open]:animate-[halo-fade_220ms_var(--ease-brand)]" />
        <D.Content
          className={cn(
            "fixed top-1/2 left-1/2 z-50 flex max-h-[85dvh] -translate-x-1/2 -translate-y-1/2 flex-col",
            "rounded-xl border border-border-strong bg-surface shadow-lg outline-none",
            "data-[state=open]:animate-[halo-dialog_220ms_var(--ease-brand)]",
            width === "default" ? "w-[min(560px,calc(100vw-32px))]" : "w-[min(800px,calc(100vw-32px))]",
          )}
        >
          <div className="flex items-start justify-between gap-4 px-6 pt-6 pb-4 sm:px-8 sm:pt-8">
            <div className="flex flex-col gap-2">
              <D.Title className="font-display text-h3 text-fg">{title}</D.Title>
              {description ? (
                <D.Description className="text-body text-fg-3">{description}</D.Description>
              ) : (
                <VisuallyHidden.Root asChild>
                  <D.Description>{title}</D.Description>
                </VisuallyHidden.Root>
              )}
            </div>
            <D.Close aria-label="Close" className="-mt-1 -mr-2 grid size-8 place-items-center rounded-md text-fg-3 hover:bg-hover hover:text-fg">
              <X aria-hidden="true" size={16} strokeWidth={1.75} />
            </D.Close>
          </div>
          {children ? <div className="flex-1 overflow-y-auto px-6 pb-4 sm:px-8">{children}</div> : null}
          {footer ? <div className="flex justify-end gap-2 px-6 pt-4 pb-6 sm:px-8 sm:pb-8">{footer}</div> : null}
        </D.Content>
      </D.Portal>
    </D.Root>
  );
}
