"use client";

import { ArrowUpRight, LayoutGrid, Search, SearchX } from "lucide-react";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { EmptyState } from "@/components/ui/empty-state";
import { Input } from "@/components/ui/input";

export function AppLauncher({ apps }: { apps: { id: string; name: string; homepage: string; launchUrl: string; host: string }[] }) {
  const [query, setQuery] = useState("");
  const needle = query.trim().toLowerCase();
  const shown = needle ? apps.filter((app) => `${app.name} ${app.host}`.toLowerCase().includes(needle)) : apps;

  if (apps.length === 0) {
    return <EmptyState icon={LayoutGrid} title="No applications yet" description="When an administrator gives you access to an application, it appears here." />;
  }

  return (
    <div className="flex flex-col gap-6">
      {apps.length > 6 ? (
        <div className="sm:w-80">
          <Input
            type="search"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="Search applications"
            aria-label="Search applications"
            leading={<Search size={16} strokeWidth={1.75} />}
          />
        </div>
      ) : null}
      {shown.length > 0 ? (
        <ul className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3">
          {shown.map((app) => (
            <li key={app.id}>
              <a
                href={app.launchUrl}
                target="_blank"
                rel="noreferrer"
                className="group flex items-center gap-4 rounded-lg border border-border bg-surface p-4 transition-colors duration-fast ease-brand hover:border-border-strong hover:bg-hover"
              >
                <span aria-hidden="true" className="grid size-10 shrink-0 place-items-center rounded-md border border-border-strong bg-n800 font-display text-h4 text-fg-2">
                  {app.name[0]}
                </span>
                <span className="flex min-w-0 flex-1 flex-col">
                  <span className="truncate text-body-sm font-semibold text-fg">{app.name}</span>
                  <span className="truncate text-caption text-fg-3">{app.host}</span>
                </span>
                <ArrowUpRight aria-hidden="true" size={16} strokeWidth={1.75} className="shrink-0 text-fg-3 transition-colors duration-fast ease-brand group-hover:text-fg" />
                <span className="sr-only">(opens in a new tab)</span>
              </a>
            </li>
          ))}
        </ul>
      ) : (
        <EmptyState
          icon={SearchX}
          title="No applications match"
          description="Check the spelling. If something you need isn't listed, ask an administrator for access."
          action={
            <Button size="sm" onClick={() => setQuery("")}>
              Clear search
            </Button>
          }
        />
      )}
    </div>
  );
}
