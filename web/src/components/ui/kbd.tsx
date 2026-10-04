import type { ReactNode } from "react";
import { cn } from "@/lib/cn";

export function Kbd({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <kbd
      className={cn(
        "inline-grid h-5 min-w-5 place-items-center rounded-sm border border-border-strong bg-n800 px-1 font-mono text-caption leading-none text-fg-3",
        className,
      )}
    >
      {children}
    </kbd>
  );
}
