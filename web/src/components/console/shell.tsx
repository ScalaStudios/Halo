"use client";

import { usePathname } from "next/navigation";
import { Dialog as D, VisuallyHidden } from "radix-ui";
import { ChevronRight, LogOut, Menu as MenuIcon, UserRound } from "lucide-react";
import { useEffect, useState, type ReactNode } from "react";
import { Avatar } from "@/components/ui/avatar";
import { Menu, MenuContent, MenuItem, MenuLabel, MenuSeparator, MenuTrigger } from "@/components/ui/menu";
import { api } from "@/lib/api/client";
import { CommandPalette } from "./command-palette";
import { findItem, nav } from "./nav";
import { Sidebar } from "./sidebar";
import { useMutation } from "./use-mutation";

export function ConsoleShell({ user, orgName, children }: { user: { name: string; email: string }; orgName: string; children: ReactNode }) {
  const pathname = usePathname();
  const { run, router } = useMutation();
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const item = findItem(pathname);
  const group = nav.find((g) => g.items.some((i) => i.href === item?.href));

  function signOut() {
    void run("sign-out", () => api("/auth/sign-out", { method: "POST" }), () => router.push("/sign-in"));
  }

  useEffect(() => {
    function onKey(event: KeyboardEvent) {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setPaletteOpen((open) => !open);
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  return (
    <div className="flex min-h-dvh">
      <aside className="sticky top-0 hidden h-dvh w-66 shrink-0 border-r border-border bg-canvas lg:block">
        <Sidebar orgName={orgName} onSearch={() => setPaletteOpen(true)} />
      </aside>

      <D.Root open={drawerOpen} onOpenChange={setDrawerOpen}>
        <D.Portal>
          <D.Overlay className="fixed inset-0 z-40 bg-ink/80 lg:hidden" />
          <D.Content className="fixed inset-y-0 left-0 z-50 w-[min(288px,85vw)] border-r border-border bg-canvas shadow-lg outline-none data-[state=open]:animate-[halo-fade_220ms_var(--ease-brand)] lg:hidden">
            <VisuallyHidden.Root asChild>
              <D.Title>Navigation</D.Title>
            </VisuallyHidden.Root>
            <Sidebar
              orgName={orgName}
              onSearch={() => {
                setDrawerOpen(false);
                setPaletteOpen(true);
              }}
              onNavigate={() => setDrawerOpen(false)}
            />
          </D.Content>
        </D.Portal>
      </D.Root>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="sticky top-0 z-30 flex h-16 shrink-0 items-center gap-3 border-b border-border bg-canvas px-5 sm:px-8 lg:px-12">
          <button
            type="button"
            onClick={() => setDrawerOpen(true)}
            aria-label="Open navigation"
            className="-ml-2 grid size-10 place-items-center rounded-md text-fg-3 hover:bg-hover hover:text-fg lg:hidden"
          >
            <MenuIcon aria-hidden="true" size={20} strokeWidth={1.75} />
          </button>
          <nav aria-label="Breadcrumb" className="flex min-w-0 items-center gap-2 text-body-sm">
            {group?.label && group.label !== item?.label ? (
              <>
                <span className="hidden truncate text-fg-3 sm:inline">{group.label}</span>
                <ChevronRight aria-hidden="true" size={16} strokeWidth={1.75} className="hidden shrink-0 text-n600 sm:block" />
              </>
            ) : null}
            <span className="truncate font-medium text-fg">{item?.label ?? "Overview"}</span>
          </nav>
          <div className="ml-auto flex items-center gap-2">
            <Menu>
              <MenuTrigger
                aria-label="Account menu"
                className="flex h-10 items-center gap-2 rounded-md px-2 text-body-sm text-fg-2 transition-colors duration-fast hover:bg-hover hover:text-fg data-[state=open]:bg-press"
              >
                <Avatar name={user.name} size="sm" />
                <span className="hidden sm:inline">{user.name}</span>
              </MenuTrigger>
              <MenuContent className="w-64">
                <MenuLabel>
                  <span className="block truncate text-body-sm font-semibold text-fg">{user.name}</span>
                  <span className="block truncate font-normal">{user.email}</span>
                </MenuLabel>
                <MenuSeparator />
                <MenuItem icon={UserRound} onSelect={() => router.push("/account")}>
                  Your Halo account
                </MenuItem>
                <MenuItem icon={LogOut} onSelect={signOut}>
                  Sign out
                </MenuItem>
              </MenuContent>
            </Menu>
          </div>
        </header>
        <main id="main" className="w-full max-w-360 flex-1 px-5 py-8 sm:px-8 lg:px-12">
          {children}
        </main>
      </div>

      <CommandPalette open={paletteOpen} onOpenChange={setPaletteOpen} />
    </div>
  );
}
