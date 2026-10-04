"use client";

import { useRouter } from "next/navigation";
import { Dialog as D, VisuallyHidden } from "radix-ui";
import { Command } from "cmdk";
import { AppWindow, CornerDownLeft, Plus, Search, UserPlus } from "lucide-react";
import { useEffect, useState, type ReactNode } from "react";
import { Avatar } from "@/components/ui/avatar";
import { api } from "@/lib/api/client";
import type { Application, User } from "@/lib/types";
import { BASE, allItems } from "./nav";

const groupClass =
  "[&_[cmdk-group-heading]]:px-2 [&_[cmdk-group-heading]]:pt-3 [&_[cmdk-group-heading]]:pb-1 [&_[cmdk-group-heading]]:text-label [&_[cmdk-group-heading]]:text-fg-3";

function Item({ value, onSelect, children }: { value: string; onSelect: () => void; children: ReactNode }) {
  return (
    <Command.Item
      value={value}
      onSelect={onSelect}
      className="group flex h-10 cursor-pointer items-center gap-3 rounded-md px-2 text-body-sm text-fg-2 data-[selected=true]:bg-press data-[selected=true]:text-fg"
    >
      {children}
      <CornerDownLeft aria-hidden="true" size={16} strokeWidth={1.75} className="ml-auto text-fg-3 opacity-0 group-data-[selected=true]:opacity-100" />
    </Command.Item>
  );
}

export function CommandPalette({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const router = useRouter();
  const [results, setResults] = useState<{ users: User[]; applications: Application[] } | null>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!open || results) return;
    setLoading(true);
    Promise.all([api<User[]>("/users"), api<Application[]>("/applications")])
      .then(([users, applications]) => setResults({ users, applications }))
      .catch(() => {})
      .finally(() => setLoading(false));
  }, [open, results]);

  function go(href: string) {
    onOpenChange(false);
    router.push(href);
  }

  return (
    <D.Root open={open} onOpenChange={onOpenChange}>
      <D.Portal>
        <D.Overlay className="fixed inset-0 z-40 bg-ink/80 data-[state=open]:animate-[halo-fade_160ms_var(--ease-brand)]" />
        <D.Content className="fixed top-[12dvh] left-1/2 z-50 w-[min(640px,calc(100vw-32px))] -translate-x-1/2 overflow-hidden rounded-xl border border-border-strong bg-surface shadow-lg outline-none data-[state=open]:animate-[halo-dialog_220ms_var(--ease-brand)]">
          <VisuallyHidden.Root asChild>
            <D.Title>Search Halo</D.Title>
          </VisuallyHidden.Root>
          <VisuallyHidden.Root asChild>
            <D.Description>Jump to a page, user or application, or run an action.</D.Description>
          </VisuallyHidden.Root>
          <Command label="Search Halo" loop>
            <div className="flex items-center gap-3 border-b border-border px-4">
              <Search aria-hidden="true" size={16} strokeWidth={1.75} className="text-fg-3" />
              <Command.Input
                placeholder="Search users, applications and settings"
                className="h-12 flex-1 bg-transparent text-body text-fg outline-none placeholder:text-fg-3 focus-visible:shadow-none"
              />
            </div>
            <Command.List className="max-h-[min(440px,60dvh)] overflow-y-auto p-2">
              <Command.Empty className="px-2 py-8 text-center text-body-sm text-fg-3">No matches. Try a name, an email address or a page.</Command.Empty>
              <Command.Group heading="Actions" className={groupClass}>
                <Item value="Add application register oidc saml" onSelect={() => go(`${BASE}/applications/new`)}>
                  <Plus aria-hidden="true" size={16} strokeWidth={1.75} className="text-fg-3" />
                  Add application
                </Item>
                <Item value="Invite user add person" onSelect={() => go(`${BASE}/users?invite=1`)}>
                  <UserPlus aria-hidden="true" size={16} strokeWidth={1.75} className="text-fg-3" />
                  Invite user
                </Item>
              </Command.Group>
              <Command.Group heading="Go to" className={groupClass}>
                {allItems.map((item) => {
                  const Icon = item.icon;
                  return (
                    <Item key={item.href} value={`${item.group ?? ""} ${item.label}`} onSelect={() => go(item.href)}>
                      <Icon aria-hidden="true" size={16} strokeWidth={1.75} className="text-fg-3" />
                      <span>{item.label}</span>
                      {item.group ? <span className="text-fg-3">{item.group}</span> : null}
                    </Item>
                  );
                })}
              </Command.Group>
              {loading ? (
                <Command.Loading>
                  <div className="px-2 py-3 text-body-sm text-fg-3">Loading people and applications…</div>
                </Command.Loading>
              ) : null}
              <Command.Group heading="Users" className={groupClass}>
                {results?.users.map((user) => (
                  <Item key={user.id} value={`user ${user.name} ${user.email}`} onSelect={() => go(`${BASE}/users/${user.id}`)}>
                    <Avatar name={user.name} size="sm" />
                    <span className="truncate">{user.name}</span>
                    <span className="truncate text-fg-3">{user.email}</span>
                  </Item>
                ))}
              </Command.Group>
              <Command.Group heading="Applications" className={groupClass}>
                {results?.applications.map((app) => (
                  <Item key={app.id} value={`application ${app.name}`} onSelect={() => go(`${BASE}/applications/${app.id}`)}>
                    <AppWindow aria-hidden="true" size={16} strokeWidth={1.75} className="text-fg-3" />
                    <span className="truncate">{app.name}</span>
                  </Item>
                ))}
              </Command.Group>
            </Command.List>
          </Command>
        </D.Content>
      </D.Portal>
    </D.Root>
  );
}
