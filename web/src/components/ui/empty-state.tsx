import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";
import { cn } from "@/lib/cn";

export function EmptyState({
  icon: Icon,
  title,
  description,
  action,
  className,
}: {
  icon?: LucideIcon;
  title: string;
  description?: ReactNode;
  action?: ReactNode;
  className?: string;
}) {
  return (
    <div className={cn("flex flex-col items-center gap-3 px-6 py-16 text-center", className)}>
      {Icon ? (
        <span className="mb-1 grid size-12 place-items-center rounded-lg border border-border bg-sunken text-fg-3">
          <Icon aria-hidden="true" size={24} strokeWidth={1.75} />
        </span>
      ) : null}
      <p className="text-h4 text-fg">{title}</p>
      {description ? <div className="max-w-md text-body-sm text-fg-3">{description}</div> : null}
      {action ? <div className="mt-2">{action}</div> : null}
    </div>
  );
}
