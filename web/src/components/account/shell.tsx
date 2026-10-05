"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { History, KeyRound, LayoutGrid, LogOut, MonitorSmartphone, ShieldCheck, UserRound, UserRoundCog } from "lucide-react";
import type { ReactNode } from "react";
import { HaloLogo } from "@/components/halo-mark";
import { Avatar } from "@/components/ui/avatar";
import { Menu, MenuContent, MenuItem, MenuLabel, MenuSeparator, MenuTrigger } from "@/components/ui/menu";
import { TabNav } from "@/components/ui/tab-nav";
import { useToast } from "@/components/ui/toast";
import { describeFailure } from "@/components/auth/passkey";
import { api } from "@/lib/api/client";
import { cn } from "@/lib/cn";
import type { User } from "@/lib/types";

const nav = [
  { label: "Account", href: "/account", icon: UserRound },
  { label: "Security", href: "/account/security", icon: ShieldCheck },
  { label: "Sessions", href: "/account/sessions", icon: MonitorSmartphone },
  { label: "Applications", href: "/account/applications", icon: LayoutGrid },
  { label: "Access", href: "/account/access", icon: KeyRound },
  { label: "Activity", href: "/account/activity", icon: History },
];

export function AccountShell({ user, children }: { user: Pick<User, "name" | "email" | "roles" | "avatarUrl">; children: ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const toast = useToast();
  const path = pathname.startsWith("/account/reviews") ? "/account/access" : pathname;
  const active = [...nav].reverse().find((item) => path === item.href || path.startsWith(`${item.href}/`))?.href ?? "/account";

  async function signOut() {
    try {
      await api("/auth/sign-out", { method: "POST" });
      window.location.assign("/sign-in");
    } catch (error) {
      toast({ title: "Couldn't sign you out", description: describeFailure(error), tone: "danger" });
    }
  }

  return (
    <div className="flex min-h-dvh flex-col">
      <header className="sticky top-0 z-30 border-b border-border bg-canvas">
        <div className="mx-auto flex h-16 w-full max-w-288 items-center gap-3 px-5 sm:px-8">
          <Link href="/account" className="flex items-center gap-3 rounded-md">
            <HaloLogo />
            <span aria-hidden="true" className="h-6 w-px bg-border" />
            <span className="text-body text-fg-3">Account</span>
          </Link>
          <Menu>
            <MenuTrigger
              aria-label="Account menu"
              className="ml-auto flex h-10 items-center gap-2 rounded-md px-2 text-body-sm text-fg-2 transition-colors duration-fast ease-brand hover:bg-hover hover:text-fg data-[state=open]:bg-press"
            >
              <Avatar name={user.name} src={user.avatarUrl} size="sm" />
              <span className="hidden sm:inline">{user.name}</span>
            </MenuTrigger>
            <MenuContent className="w-64">
              <MenuLabel>
                <span className="block truncate text-body-sm font-semibold text-fg">{user.name}</span>
                <span className="block truncate font-normal">{user.email}</span>
              </MenuLabel>
              <MenuSeparator />
              {user.roles.length > 0 ? (
                <MenuItem icon={UserRoundCog} onSelect={() => router.push("/admin")}>
                  Halo administration
                </MenuItem>
              ) : null}
              <MenuItem icon={LogOut} onSelect={signOut}>
                Sign out
              </MenuItem>
            </MenuContent>
          </Menu>
        </div>
      </header>
      <TabNav
        label="Account"
        tabs={nav.map((item) => ({ id: item.href, label: item.label }))}
        active={active}
        hrefFor={(href) => href}
        className="px-5 sm:px-8 lg:hidden"
      />

      <div className="mx-auto flex w-full max-w-288 flex-1 gap-12 px-5 sm:px-8">
        <nav aria-label="Account" className="sticky top-16 hidden h-fit w-56 shrink-0 py-12 lg:block">
          <ul className="flex flex-col gap-1">
            {nav.map((item) => {
              const current = item.href === active;
              const Icon = item.icon;
              return (
                <li key={item.href}>
                  <Link
                    href={item.href}
                    aria-current={current ? "page" : undefined}
                    className={cn(
                      "relative flex h-10 items-center gap-3 rounded-md px-3 text-nav transition-colors duration-fast ease-brand",
                      current ? "bg-press text-fg" : "text-fg-3 hover:bg-hover hover:text-fg",
                    )}
                  >
                    {current ? <span aria-hidden="true" className="absolute top-2 bottom-2 -left-3 w-0.5 rounded-full bg-ember" /> : null}
                    <Icon aria-hidden="true" size={16} strokeWidth={1.75} className={current ? "text-ember" : undefined} />
                    {item.label}
                  </Link>
                </li>
              );
            })}
          </ul>
        </nav>
        <main id="main" className="min-w-0 max-w-200 flex-1 py-8 sm:py-12">
          {children}
        </main>
      </div>
    </div>
  );
}
