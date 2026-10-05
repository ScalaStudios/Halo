import type { Metadata } from "next";
import Link from "next/link";
import { ChevronRight, LayoutGrid, MonitorSmartphone, ShieldCheck } from "lucide-react";
import { PageHeader } from "@/components/ui/page-header";
import { AvatarEditor } from "@/components/account/avatar-editor";
import { Card } from "@/components/ui/card";
import { DescriptionList } from "@/components/ui/description-list";
import { apiGet } from "@/lib/api/server";
import { pluralize } from "@/lib/format";
import { STRENGTH } from "@/lib/labels";
import type { Organization, Session, User } from "@/lib/types";

export const metadata: Metadata = { title: { absolute: "Halo account" } };

export default async function AccountPage() {
  const [user, org, sessions, apps] = await Promise.all([
    apiGet<User>("/me"),
    apiGet<Organization>("/organization"),
    apiGet<Session[]>("/me/sessions"),
    apiGet<unknown[]>("/me/applications"),
  ]);
  const count = (kind: string) => user.methods.filter((m) => m.kind === kind).length;
  const strong = count("passkey") + count("security-key");
  const resistant = user.strength === "phishing-resistant";
  const links = [
    { href: "/account/sessions", icon: MonitorSmartphone, label: "Sessions", detail: `${pluralize(sessions.length, "device")} signed in` },
    { href: "/account/applications", icon: LayoutGrid, label: "Applications", detail: `${pluralize(apps.length, "application")} available to you` },
  ];

  return (
    <div className="flex animate-page flex-col gap-12">
      <PageHeader size="large" title="Account" description={`Your profile, how you sign in and what you can open at ${org.name}.`} />

      <div className="flex flex-col gap-8">
        <Card className="flex flex-col gap-6 p-6">
          <AvatarEditor name={user.name} avatarUrl={user.avatarUrl} endpoint="/api/v1/me/avatar">
            <div className="flex min-w-0 flex-col gap-1">
              <h2 className="truncate text-h3 text-fg">{user.name}</h2>
              <p className="truncate text-body-sm text-fg-3">{user.email}</p>
            </div>
          </AvatarEditor>
          <DescriptionList
            columns={2}
            items={[
              { label: "Title", value: user.title || "Not set" },
              { label: "Department", value: user.department || "Not set" },
              { label: "Location", value: user.location || "Not set" },
            ]}
          />
          <p className="border-t border-border pt-4 text-caption text-fg-3">
            Your profile is managed by {org.name} IT. To correct something, email{" "}
            <a href="mailto:it@example.com" className="text-link underline underline-offset-3 hover:text-ember-hover">
              it@example.com
            </a>
            .
          </p>
        </Card>

        <Card className="flex flex-col gap-4 p-6 sm:flex-row sm:items-center">
          <span className="grid size-10 shrink-0 place-items-center rounded-md border border-border bg-sunken">
            <ShieldCheck aria-hidden="true" size={20} strokeWidth={1.75} className={resistant ? "text-success" : "text-warning"} />
          </span>
          <div className="flex min-w-0 flex-1 flex-col gap-1">
            <h2 className="text-h4 text-fg">
              {resistant ? "You sign in with a passkey." : "You don't have a passkey yet."} {STRENGTH[user.strength].label}.
            </h2>
            <p className="text-body-sm text-fg-3">
              {pluralize(count("passkey"), "passkey")} and {pluralize(count("security-key"), "security key")} enrolled.{" "}
              {strong > 1
                ? "Losing one device won't lock you out."
                : strong === 1
                  ? "Add another so losing one device won't lock you out."
                  : "Add one to sign in without codes."}
            </p>
          </div>
          <Link href="/account/security" className="shrink-0 text-body-sm text-link underline underline-offset-3 hover:text-ember-hover">
            Review security
          </Link>
        </Card>

        <ul className="overflow-hidden rounded-lg border border-border bg-surface">
          {links.map((link) => {
            const Icon = link.icon;
            return (
              <li key={link.href} className="border-t border-border first:border-t-0">
                <Link href={link.href} className="flex items-center gap-4 px-6 py-4 transition-colors duration-fast ease-brand hover:bg-hover">
                  <Icon aria-hidden="true" size={20} strokeWidth={1.75} className="shrink-0 text-fg-3" />
                  <span className="flex min-w-0 flex-1 flex-col">
                    <span className="text-body-sm font-semibold text-fg">{link.label}</span>
                    <span className="tnum text-body-sm text-fg-3">{link.detail}</span>
                  </span>
                  <ChevronRight aria-hidden="true" size={16} strokeWidth={1.75} className="shrink-0 text-fg-3" />
                </Link>
              </li>
            );
          })}
        </ul>
      </div>
    </div>
  );
}
