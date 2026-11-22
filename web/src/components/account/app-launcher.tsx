"use client";

import { ArrowLeft, ArrowRight, ArrowUpRight, Ellipsis, LayoutGrid, Search, SearchX, Star, StarOff } from "lucide-react";
import { useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { Avatar } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { EmptyState } from "@/components/ui/empty-state";
import { Input } from "@/components/ui/input";
import { Menu, MenuContent, MenuItem, MenuSeparator, MenuTrigger } from "@/components/ui/menu";
import { SectionTitle } from "@/components/ui/section-title";
import { api } from "@/lib/api/client";
import { accountMessages } from "@/lib/i18n/account";
import { useLocale } from "@/lib/i18n/client";

type LauncherApp = { id: string; name: string; homepage: string; launchUrl: string; host: string; logoUrl: string | null; favourite: boolean; roles: string[] };
type Section = { id: string; title: string | null; apps: LauncherApp[] };

export function AppLauncher({ apps, collections }: { apps: LauncherApp[]; collections: { id: string; name: string; applicationIds: string[] }[] }) {
  const copy = accountMessages(useLocale());
  const t = copy.applications;
  const [query, setQuery] = useState("");
  const { pending, run } = useMutation();
  const needle = query.trim().toLowerCase();
  const favourites = apps.filter((app) => app.favourite);
  const collected = new Set(collections.flatMap((c) => c.applicationIds));

  if (apps.length === 0) {
    return <EmptyState icon={LayoutGrid} title={t.empty} description={t.emptyHint} />;
  }

  const sections: Section[] = needle
    ? [{ id: "results", title: null, apps: apps.filter((app) => `${app.name} ${app.host}`.toLowerCase().includes(needle)) }]
    : [
        { id: "favourites", title: t.favourites, apps: favourites },
        ...collections.map((c) => ({ id: c.id, title: c.name, apps: apps.filter((app) => c.applicationIds.includes(app.id)) })),
        {
          id: "rest",
          title: collections.length ? t.other : favourites.length ? t.all : null,
          apps: apps.filter((app) => !collected.has(app.id)),
        },
      ].filter((section) => section.apps.length > 0);

  function arrange(key: string, order: string[], favouriteIds: string[]) {
    void run(key, () => api("/me/applications", { method: "PUT", body: { order, favourites: favouriteIds } }));
  }

  function toggleFavourite(app: LauncherApp) {
    const ids = favourites.map((f) => f.id);
    arrange(app.id, apps.map((a) => a.id), app.favourite ? ids.filter((id) => id !== app.id) : [...ids, app.id]);
  }

  function move(app: LauncherApp, neighbour: LauncherApp) {
    const order = apps.map((a) => (a.id === app.id ? neighbour.id : a.id === neighbour.id ? app.id : a.id));
    arrange(app.id, order, favourites.map((f) => f.id));
  }

  return (
    <div className="flex flex-col gap-8">
      {apps.length > 6 ? (
        <div className="sm:w-80">
          <Input
            type="search"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder={t.search}
            aria-label={t.search}
            leading={<Search size={16} strokeWidth={1.75} />}
          />
        </div>
      ) : null}
      {sections.length > 0 ? (
        sections.map((section) => (
          <section key={section.id} aria-label={section.title ?? copy.nav.applications} className="flex flex-col gap-4">
            {section.title ? <SectionTitle title={section.title} /> : null}
            <ul className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3">
              {section.apps.map((app, index) => {
                const before = section.apps[index - 1];
                const after = section.apps[index + 1];
                return (
                  <li
                    key={app.id}
                    className="flex items-center gap-2 rounded-lg border border-border bg-surface pr-2 transition-colors duration-fast ease-brand hover:border-border-strong hover:bg-hover"
                  >
                    <a href={app.launchUrl} target="_blank" rel="noreferrer" className="group flex min-w-0 flex-1 items-center gap-4 rounded-lg p-4">
                      <Avatar name={app.name} src={app.logoUrl} size="lg" square />
                      <span className="flex min-w-0 flex-1 flex-col">
                        <span className="flex min-w-0 items-center gap-1">
                          <span className="truncate text-body-sm font-semibold text-fg">{app.name}</span>
                          {app.favourite ? (
                            <>
                              <Star aria-hidden="true" size={16} strokeWidth={1.75} fill="currentColor" className="shrink-0 text-ember" />
                              <span className="sr-only">{t.favourite}</span>
                            </>
                          ) : null}
                        </span>
                        <span className="truncate text-caption text-fg-3">{app.host}</span>
                        {app.roles.length ? (
                          <span className="truncate text-caption text-fg-2">
                            <span className="sr-only">{t.yourRoles}</span>
                            {app.roles.join(", ")}
                          </span>
                        ) : null}
                      </span>
                      <ArrowUpRight aria-hidden="true" size={16} strokeWidth={1.75} className="shrink-0 text-fg-3 transition-colors duration-fast ease-brand group-hover:text-fg" />
                      <span className="sr-only">{copy.newTab}</span>
                    </a>
                    <Menu>
                      <MenuTrigger asChild>
                        <Button size="icon-sm" variant="quiet" aria-label={t.options(app.name)} loading={pending === app.id} disabled={pending !== null}>
                          <Ellipsis aria-hidden="true" size={16} strokeWidth={1.75} />
                        </Button>
                      </MenuTrigger>
                      <MenuContent>
                        <MenuItem icon={app.favourite ? StarOff : Star} onSelect={() => toggleFavourite(app)}>
                          {app.favourite ? t.removeFavourite : t.addFavourite}
                        </MenuItem>
                        <MenuSeparator />
                        <MenuItem icon={ArrowLeft} disabled={!before} onSelect={() => before && move(app, before)}>
                          {t.moveEarlier}
                        </MenuItem>
                        <MenuItem icon={ArrowRight} disabled={!after} onSelect={() => after && move(app, after)}>
                          {t.moveLater}
                        </MenuItem>
                      </MenuContent>
                    </Menu>
                  </li>
                );
              })}
            </ul>
          </section>
        ))
      ) : (
        <EmptyState
          icon={SearchX}
          title={t.noMatch}
          description={t.noMatchHint}
          action={
            <Button size="sm" onClick={() => setQuery("")}>
              {t.clearSearch}
            </Button>
          }
        />
      )}
    </div>
  );
}
