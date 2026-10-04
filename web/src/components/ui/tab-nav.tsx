import Link from "next/link";
import { cn } from "@/lib/cn";

export type TabItem = { id: string; label: string; count?: number };

export function TabNav({
  tabs,
  active,
  hrefFor,
  label,
  className,
}: {
  tabs: TabItem[];
  active: string;
  hrefFor: (id: string) => string;
  label: string;
  className?: string;
}) {
  return (
    <nav aria-label={label} className={cn("overflow-x-auto overflow-y-hidden border-b border-border [scrollbar-width:none]", className)}>
      <ul className="flex min-w-max gap-1">
        {tabs.map((tab) => {
          const current = tab.id === active;
          return (
            <li key={tab.id}>
              <Link
                href={hrefFor(tab.id)}
                scroll={false}
                aria-current={current ? "page" : undefined}
                className={cn(
                  "relative flex h-12 items-center gap-2 rounded-t-md px-3 text-body-sm font-medium transition-colors duration-fast ease-brand",
                  current ? "text-fg" : "text-fg-3 hover:text-fg",
                )}
              >
                {tab.label}
                {tab.count !== undefined ? <span className="tnum text-caption text-fg-3">{tab.count}</span> : null}
                {current ? <span aria-hidden="true" className="absolute inset-x-3 bottom-0 h-0.5 rounded-full bg-ember" /> : null}
              </Link>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}
