import type { ReactNode } from "react";
import { cn } from "@/lib/cn";

export type DescriptionItem = { label: string; value: ReactNode; mono?: boolean };

export function DescriptionList({
  items,
  columns = 1,
  className,
}: {
  items: DescriptionItem[];
  columns?: 1 | 2;
  className?: string;
}) {
  return (
    <dl className={cn("grid gap-x-8 gap-y-4", columns === 2 && "sm:grid-cols-2", className)}>
      {items.map((item) => (
        <div key={item.label} className="flex min-w-0 flex-col gap-1">
          <dt className="text-label text-fg-3">{item.label}</dt>
          <dd className={cn("min-w-0 text-body-sm text-fg", item.mono && "font-mono text-code-sm break-all")}>
            {item.value}
          </dd>
        </div>
      ))}
    </dl>
  );
}
