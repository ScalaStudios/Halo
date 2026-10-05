"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { ChevronDown, Search } from "lucide-react";
import { useEffect, useState } from "react";
import { HaloMark } from "@/components/halo-mark";
import { Kbd } from "@/components/ui/kbd";
import { cn } from "@/lib/cn";
import { BASE, findItem, nav } from "./nav";

const STORAGE_KEY = "halo.sidebar.collapsed";

export function Sidebar({ orgName, onSearch, onNavigate }: { orgName: string; onSearch: () => void; onNavigate?: () => void }) {
  const pathname = usePathname();
  const activeHref = findItem(pathname)?.href;
  const activeGroup = nav.find((group) => group.items.some((item) => item.href === activeHref))?.id;
  const [collapsed, setCollapsed] = useState<string[]>([]);

  useEffect(() => {
    try {
      const stored = localStorage.getItem(STORAGE_KEY);
      if (stored) setCollapsed(JSON.parse(stored) as string[]);
    } catch {
      setCollapsed([]);
    }
  }, []);

  function toggle(id: string) {
    setCollapsed((current) => {
      const next = current.includes(id) ? current.filter((value) => value !== id) : [...current, id];
      try {
        localStorage.setItem(STORAGE_KEY, JSON.stringify(next));
      } catch {}
      return next;
    });
  }

  return (
    <div className="flex h-full flex-col">
      <div className="flex h-16 shrink-0 items-center gap-3 border-b border-border px-4">
        <Link href={BASE} onClick={onNavigate} className="flex min-w-0 flex-1 items-center gap-3 rounded-md">
          <HaloMark className="size-6 shrink-0" />
          <span className="flex min-w-0 flex-col">
            <span className="truncate font-display text-h4 text-fg">Halo</span>
            <span className="truncate text-caption text-fg-3">{orgName}</span>
          </span>
        </Link>
      </div>

      <div className="px-3 pt-3">
        <button
          type="button"
          onClick={onSearch}
          className="flex h-8 w-full items-center gap-2 rounded-md border border-border bg-sunken px-3 text-body-sm text-fg-3 transition-colors duration-fast hover:border-border-strong hover:text-fg-2"
        >
          <Search aria-hidden="true" size={16} strokeWidth={1.75} />
          <span className="flex-1 text-left">Search</span>
          <Kbd>⌘K</Kbd>
        </button>
      </div>

      <nav aria-label="Administration" className="flex-1 overflow-y-auto px-3 pt-2 pb-4">
        {nav.map((group) => {
          const isCollapsed = group.label !== null && collapsed.includes(group.id) && group.id !== activeGroup;
          return (
            <div key={group.id} className="pt-2">
              {group.label ? (
                <button
                  type="button"
                  onClick={() => toggle(group.id)}
                  aria-expanded={!isCollapsed}
                  className="group flex h-8 w-full items-center justify-between rounded-md px-2 text-label text-fg-3 transition-colors duration-fast hover:text-fg-2"
                >
                  {group.label}
                  <ChevronDown
                    aria-hidden="true"
                    size={16}
                    strokeWidth={1.75}
                    className={cn(
                      "text-n500 opacity-0 transition-[transform,opacity] duration-fast ease-brand group-hover:opacity-100 group-focus-visible:opacity-100",
                      isCollapsed && "-rotate-90 opacity-100",
                    )}
                  />
                </button>
              ) : null}
              {isCollapsed ? null : (
                <ul className="flex flex-col gap-px">
                  {group.items.map((item) => {
                    const active = item.href === activeHref;
                    const Icon = item.icon;
                    return (
                      <li key={item.href}>
                        <Link
                          href={item.href}
                          onClick={onNavigate}
                          aria-current={active ? "page" : undefined}
                          className={cn(
                            "relative flex h-8 items-center gap-3 rounded-md px-2 text-body-sm font-medium transition-colors duration-fast ease-brand",
                            active ? "bg-press text-fg" : "text-fg-3 hover:bg-hover hover:text-fg",
                          )}
                        >
                          {active ? <span aria-hidden="true" className="absolute top-2 bottom-2 -left-3 w-0.5 rounded-full bg-ember" /> : null}
                          <Icon aria-hidden="true" size={16} strokeWidth={1.75} className={active ? "text-ember" : undefined} />
                          <span className="truncate">{item.label}</span>
                        </Link>
                      </li>
                    );
                  })}
                </ul>
              )}
            </div>
          );
        })}
      </nav>

      <div className="flex h-12 shrink-0 items-center justify-between border-t border-border px-4 text-caption text-fg-3">
        <span className="font-mono">v0.1.1</span>
        <Link href={`${BASE}/developers`} onClick={onNavigate} className="transition-colors duration-fast hover:text-fg">
          Documentation
        </Link>
      </div>
    </div>
  );
}
