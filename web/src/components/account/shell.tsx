"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { FileCheck, History, KeyRound, LayoutGrid, LogOut, MonitorSmartphone, ShieldCheck, UserRound, UserRoundCog, UsersRound, VenetianMask, type LucideIcon } from "lucide-react";
import { useState, useTransition, type ReactNode } from "react";
import { HaloLogo } from "@/components/halo-mark";
import { Avatar } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Menu, MenuContent, MenuItem, MenuLabel, MenuSeparator, MenuTrigger } from "@/components/ui/menu";
import { LinkProgress, NavigationProgress } from "@/components/ui/navigation-progress";
import { TabNav } from "@/components/ui/tab-nav";
import { useToast } from "@/components/ui/toast";
import { describeFailure } from "@/components/auth/passkey";
import { api } from "@/lib/api/client";
import { cn } from "@/lib/cn";
import { formatTime } from "@/lib/format";
import { accountMessages } from "@/lib/i18n/account";
import type { Locale } from "@/lib/i18n/locale";
import type { FeatureId } from "@/lib/settings-types";
import type { User } from "@/lib/types";

const pages: { label: keyof ReturnType<typeof accountMessages>["nav"]; href: string; icon: LucideIcon; feature?: FeatureId }[] = [
  { label: "account", href: "/account", icon: UserRound },
  { label: "security", href: "/account/security", icon: ShieldCheck },
  { label: "sessions", href: "/account/sessions", icon: MonitorSmartphone },
  { label: "applications", href: "/account/applications", icon: LayoutGrid },
  { label: "groups", href: "/account/groups", icon: UsersRound },
  { label: "access", href: "/account/access", icon: KeyRound, feature: "governance" },
  { label: "terms", href: "/account/terms", icon: FileCheck },
  { label: "activity", href: "/account/activity", icon: History },
];

export function AccountShell({
  user,
  featuresOff,
  locale,
  children,
}: {
  user: Pick<User, "name" | "email" | "roles" | "avatarUrl" | "impersonation">;
  featuresOff: FeatureId[];
  locale: Locale;
  children: ReactNode;
}) {
  const copy = accountMessages(locale);
  const nav = pages.filter((item) => !item.feature || !featuresOff.includes(item.feature));
  const pathname = usePathname();
  const router = useRouter();
  const toast = useToast();
  const [navigating, startNavigation] = useTransition();
  const [signingOut, setSigningOut] = useState(false);
  const path = pathname.startsWith("/account/reviews") ? "/account/access" : pathname;
  const active = [...nav].reverse().find((item) => path === item.href || path.startsWith(`${item.href}/`))?.href ?? "/account";

  async function signOut() {
    setSigningOut(true);
    try {
      await api(user.impersonation ? `/impersonations/${user.impersonation.id}/end` : "/auth/sign-out", { method: "POST" });
      window.location.assign("/sign-in");
    } catch (error) {
      setSigningOut(false);
      toast({ title: copy.signOutFailed, description: describeFailure(error), tone: "danger" });
    }
  }

  return (
    <div className="flex min-h-dvh flex-col">
      {navigating || signingOut ? <NavigationProgress /> : null}
      {user.impersonation ? (
        <div lang={locale} role="status" className="border-b border-warning/20 bg-warning-container">
          <div className="mx-auto flex w-full max-w-288 flex-col gap-3 px-5 py-3 sm:flex-row sm:items-center sm:px-8">
            <VenetianMask aria-hidden="true" size={20} strokeWidth={1.75} className="hidden shrink-0 text-warning sm:block" />
            <p className="flex-1 text-body-sm text-fg">
              <span className="font-semibold">
                {copy.impersonationNotice(user.impersonation.actor.name, user.name, formatTime(user.impersonation.expiresAt))}
              </span>{" "}
              {copy.viewOnly}
            </p>
            <Button size="sm" variant="secondary" loading={signingOut} onClick={signOut} className="self-start sm:self-center">
              {copy.endImpersonation}
            </Button>
          </div>
        </div>
      ) : null}
      <header lang={locale} className="sticky top-0 z-30 border-b border-border bg-canvas">
        <div className="mx-auto flex h-16 w-full max-w-288 items-center gap-3 px-5 sm:px-8">
          <Link href="/account" className="flex items-center gap-3 rounded-md">
            <HaloLogo />
            <span aria-hidden="true" className="h-6 w-px bg-border" />
            <span className="text-body text-fg-3">{copy.nav.account}</span>
          </Link>
          {user.impersonation ? <Badge tone="warning">{copy.impersonation}</Badge> : null}
          <Menu>
            <MenuTrigger
              aria-label={copy.menu}
              className="ml-auto flex h-10 items-center gap-2 rounded-md px-2 text-body-sm text-fg-2 transition-colors duration-fast ease-brand hover:bg-hover hover:text-fg data-[state=open]:bg-press"
            >
              <Avatar name={user.name} src={user.avatarUrl} size="sm" />
              <span className="hidden sm:inline">{user.name}</span>
            </MenuTrigger>
            <MenuContent lang={locale} className="w-64">
              <MenuLabel>
                <span className="block truncate text-body-sm font-semibold text-fg">{user.name}</span>
                <span className="block truncate font-normal">{user.email}</span>
              </MenuLabel>
              <MenuSeparator />
              {user.roles.length > 0 ? (
                <MenuItem icon={UserRoundCog} onSelect={() => startNavigation(() => router.push("/admin"))}>
                  {copy.admin}
                </MenuItem>
              ) : null}
              <MenuItem icon={LogOut} onSelect={signOut}>
                {user.impersonation ? copy.endImpersonation : copy.signOut}
              </MenuItem>
            </MenuContent>
          </Menu>
        </div>
      </header>
      <TabNav
        lang={locale}
        label={copy.nav.account}
        tabs={nav.map((item) => ({ id: item.href, label: copy.nav[item.label] }))}
        active={active}
        hrefFor={(href) => href}
        className="px-5 sm:px-8 lg:hidden"
      />

      <div className="mx-auto flex w-full max-w-288 flex-1 gap-12 px-5 sm:px-8">
        <nav lang={locale} aria-label={copy.nav.account} className="sticky top-16 hidden h-fit w-56 shrink-0 py-12 lg:block">
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
                    {copy.nav[item.label]}
                    <LinkProgress />
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
