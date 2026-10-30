"use client";

import { useRouter } from "next/navigation";
import { Dialog as D, VisuallyHidden } from "radix-ui";
import { AppWindow, CornerDownLeft, Plus, Search, UserPlus, type LucideIcon } from "lucide-react";
import { useEffect, useRef, useState, useTransition } from "react";
import { Avatar } from "@/components/ui/avatar";
import { useListbox } from "@/components/ui/listbox";
import { NavigationProgress } from "@/components/ui/navigation-progress";
import { api } from "@/lib/api/client";
import type { Application, User } from "@/lib/types";
import { BASE, allItems } from "./nav";

type Entry = { id: string; group: string; label: string; detail?: string; search: string; href: string; icon?: LucideIcon; person?: boolean };

const fixed: Entry[] = [
  { id: "add-application", group: "Actions", label: "Add application", search: "Add application register oidc saml", href: `${BASE}/applications/new`, icon: Plus },
  { id: "invite-user", group: "Actions", label: "Invite user", search: "Invite user add person", href: `${BASE}/users?invite=1`, icon: UserPlus },
  ...allItems.map((item) => ({ id: item.href, group: "Go to", label: item.label, detail: item.group ?? undefined, search: `${item.group ?? ""} ${item.label}`, href: item.href, icon: item.icon })),
];

export function CommandPalette({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const router = useRouter();
  const [results, setResults] = useState<{ users: User[]; applications: Application[] } | null>(null);
  const [loading, setLoading] = useState(false);
  const [query, setQuery] = useState("");
  const [navigating, startNavigation] = useTransition();
  const list = useRef<HTMLDivElement>(null);
  const entries: Entry[] = [
    ...fixed,
    ...(results?.users ?? []).map((user) => ({
      id: user.id,
      group: "Users",
      label: user.name,
      detail: user.email,
      search: `user ${user.name} ${user.email}`,
      href: `${BASE}/users/${user.id}`,
      person: true,
    })),
    ...(results?.applications ?? []).map((app) => ({ id: app.id, group: "Applications", label: app.name, search: `application ${app.name}`, href: `${BASE}/applications/${app.id}`, icon: AppWindow })),
  ];
  const words = query.toLowerCase().split(" ");
  const matches = entries.filter((entry) => words.every((word) => entry.search.toLowerCase().includes(word)));
  const groups = [...new Set(matches.map((entry) => entry.group))];
  const listbox = useListbox(matches, (entry) => entry.id, go);

  useEffect(() => {
    if (!open || results) return;
    setLoading(true);
    Promise.all([api<User[]>("/users"), api<Application[]>("/applications")])
      .then(([users, applications]) => setResults({ users, applications }))
      .catch(() => {})
      .finally(() => setLoading(false));
  }, [open, results]);

  function close() {
    onOpenChange(false);
    setQuery("");
    listbox.reset();
  }

  function go(entry: Entry) {
    close();
    startNavigation(() => router.push(entry.href));
  }

  return (
    <>
      <D.Root open={open} onOpenChange={(next) => (next ? onOpenChange(true) : close())}>
        <D.Portal>
          <D.Overlay className="fixed inset-0 z-40 bg-ink/80 data-[state=open]:animate-[halo-fade_160ms_var(--ease-brand)]" />
          <D.Content className="fixed top-[12dvh] left-1/2 z-50 w-[min(640px,calc(100vw-32px))] -translate-x-1/2 overflow-hidden rounded-xl border border-border-strong bg-surface shadow-lg outline-none data-[state=open]:animate-[halo-dialog_220ms_var(--ease-brand)]">
            <VisuallyHidden.Root asChild>
              <D.Title>Search Halo</D.Title>
            </VisuallyHidden.Root>
            <VisuallyHidden.Root asChild>
              <D.Description>Jump to a page, user or application, or run an action.</D.Description>
            </VisuallyHidden.Root>
            <div className="flex items-center gap-3 border-b border-border px-4">
              <Search aria-hidden="true" size={16} strokeWidth={1.75} className="text-fg-3" />
              <input
                role="combobox"
                aria-label="Search Halo"
                aria-expanded={matches.length > 0}
                aria-controls={listbox.listId}
                aria-autocomplete="list"
                aria-activedescendant={listbox.activeId}
                value={query}
                onChange={(event) => {
                  setQuery(event.target.value);
                  listbox.reset();
                  list.current?.scrollTo({ top: 0 });
                }}
                onKeyDown={listbox.onKeyDown}
                placeholder="Search users, applications and settings"
                className="h-12 flex-1 bg-transparent text-body text-fg outline-none placeholder:text-fg-3 focus-visible:shadow-none"
              />
            </div>
            <div ref={list} className="max-h-[min(440px,60dvh)] overflow-y-auto p-2">
              {matches.length ? (
                <div id={listbox.listId} role="listbox" aria-label="Search results">
                  {groups.map((group) => (
                    <div key={group} role="group" aria-labelledby={`${listbox.listId}-${group}`}>
                      <div id={`${listbox.listId}-${group}`} role="presentation" className="px-2 pt-3 pb-1 text-label text-fg-3">
                        {group}
                      </div>
                      {matches
                        .filter((entry) => entry.group === group)
                        .map((entry) => {
                          const Icon = entry.icon;
                          return (
                            <div
                              key={entry.id}
                              id={listbox.optionId(entry)}
                              role="option"
                              aria-selected={entry === listbox.highlighted}
                              onMouseMove={() => listbox.highlight(entry)}
                              onClick={() => go(entry)}
                              className="group flex h-10 cursor-pointer items-center gap-3 rounded-md px-2 text-body-sm text-fg-2 aria-selected:bg-press aria-selected:text-fg"
                            >
                              {entry.person ? <Avatar name={entry.label} size="sm" /> : Icon ? <Icon aria-hidden="true" size={16} strokeWidth={1.75} className="shrink-0 text-fg-3" /> : null}
                              <span className="truncate">{entry.label}</span>
                              {entry.detail ? <span className="truncate text-fg-3">{entry.detail}</span> : null}
                              <CornerDownLeft aria-hidden="true" size={16} strokeWidth={1.75} className="ml-auto shrink-0 text-fg-3 opacity-0 group-aria-selected:opacity-100" />
                            </div>
                          );
                        })}
                    </div>
                  ))}
                </div>
              ) : (
                <p className="px-2 py-8 text-center text-body-sm text-fg-3">No matches. Try a name, an email address or a page.</p>
              )}
              {loading ? <p className="px-2 py-3 text-body-sm text-fg-3">Loading people and applications…</p> : null}
            </div>
          </D.Content>
        </D.Portal>
      </D.Root>
      {navigating ? <NavigationProgress /> : null}
    </>
  );
}
