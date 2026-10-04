import type { ReactNode } from "react";
import { cn } from "@/lib/cn";

export function PageHeader({
  title,
  description,
  meta,
  actions,
  size = "default",
  className,
}: {
  title: ReactNode;
  description?: ReactNode;
  meta?: ReactNode;
  actions?: ReactNode;
  size?: "default" | "large";
  className?: string;
}) {
  return (
    <header className={cn("flex flex-wrap items-end justify-between gap-x-8 gap-y-4", className)}>
      <div className={cn("flex min-w-0 flex-col", size === "large" ? "gap-3" : "gap-2")}>
        <div className="flex flex-wrap items-center gap-3">
          <h1 className={cn("text-fg", size === "large" ? "text-h1" : "text-h2")}>{title}</h1>
          {meta}
        </div>
        {description ? <p className={cn("max-w-2xl text-fg-3", size === "large" ? "text-body-lg" : "text-body")}>{description}</p> : null}
      </div>
      {actions ? <div className="flex flex-wrap items-center gap-2">{actions}</div> : null}
    </header>
  );
}
