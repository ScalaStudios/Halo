"use client";

import { Tooltip as T } from "radix-ui";
import type { ReactNode } from "react";

export function TooltipProvider({ children }: { children: ReactNode }) {
  return <T.Provider delayDuration={400}>{children}</T.Provider>;
}

export function Tooltip({ label, children, side = "top" }: { label: ReactNode; children: ReactNode; side?: "top" | "bottom" | "left" | "right" }) {
  return (
    <T.Root>
      <T.Trigger asChild>{children}</T.Trigger>
      <T.Portal>
        <T.Content
          side={side}
          sideOffset={6}
          className="z-50 max-w-64 rounded-sm border border-border-strong bg-n800 px-2 py-1 text-caption text-fg shadow-md data-[state=delayed-open]:animate-[halo-fade_160ms_var(--ease-brand)]"
        >
          {label}
        </T.Content>
      </T.Portal>
    </T.Root>
  );
}
