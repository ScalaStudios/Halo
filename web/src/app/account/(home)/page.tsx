import type { Metadata } from "next";
import Link from "next/link";
import { ChevronRight, FileDown, Fingerprint, Languages, LayoutGrid, MonitorSmartphone, ShieldCheck } from "lucide-react";
import { PageHeader } from "@/components/ui/page-header";
import { AvatarEditor } from "@/components/account/avatar-editor";
import { LanguagePicker } from "@/components/account/language-picker";
import { MyAttributesButton } from "@/components/account/my-attributes";
import { ExportButton } from "@/components/settings/export-button";
import { buttonClasses } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { DescriptionList } from "@/components/ui/description-list";
import { apiGet } from "@/lib/api/server";
import { formatAttribute, formatDateTime } from "@/lib/format";
import { accountMessages } from "@/lib/i18n/account";
import { requestLocale } from "@/lib/i18n/server";
import type { Attribute, Organization, Session, User } from "@/lib/types";

export async function generateMetadata(): Promise<Metadata> {
  return { title: { absolute: accountMessages(await requestLocale()).title } };
}

export default async function AccountPage() {
  const [user, org, sessions, apps, attributes, locale] = await Promise.all([
    apiGet<User>("/me"),
    apiGet<Organization>("/organization"),
    apiGet<Session[]>("/me/sessions"),
    apiGet<unknown[]>("/me/applications"),
    apiGet<Attribute[]>("/attributes"),
    requestLocale(),
  ]);
  const copy = accountMessages(locale);
  const editable = attributes.filter((a) => a.selfService);
  const count = (kind: string) => user.methods.filter((m) => m.kind === kind).length;
  const strong = count("passkey") + count("security-key");
  const resistant = user.strength === "phishing-resistant";
  const passwordSession = strong === 0 && sessions.some((s) => s.current && s.method === "password");
  const links = [
    { href: "/account/sessions", icon: MonitorSmartphone, label: copy.nav.sessions, detail: copy.signedInDevices(sessions.length) },
    { href: "/account/applications", icon: LayoutGrid, label: copy.nav.applications, detail: copy.availableApps(apps.length) },
  ];

  return (
    <div lang={locale} className="flex flex-col gap-12">
      <PageHeader size="large" title={copy.nav.account} description={copy.intro(org.name)} />

      <div className="flex flex-col gap-8">
        {passwordSession ? (
          <Card className="flex flex-col gap-4 p-6 sm:flex-row sm:items-center">
            <span className="grid size-10 shrink-0 place-items-center rounded-md border border-border bg-sunken">
              <Fingerprint aria-hidden="true" size={20} strokeWidth={1.75} className="text-ember" />
            </span>
            <div className="flex min-w-0 flex-1 flex-col gap-1">
              <h2 className="text-h4 text-fg">{copy.signedInPassword}</h2>
              <p className="text-body-sm text-fg-3">{copy.passkeyHint}</p>
            </div>
            <Link href="/account/security" className={buttonClasses("primary", "md")}>
              {copy.addPasskey}
            </Link>
          </Card>
        ) : null}

        <Card className="flex flex-col gap-6 p-6">
          <AvatarEditor locale={locale} name={user.name} avatarUrl={user.avatarUrl} endpoint="/api/v1/me/avatar">
            <div className="flex min-w-0 flex-col gap-1">
              <h2 className="flex min-w-0 items-center gap-3 text-h3 text-fg">
                <span className="truncate">{user.name}</span>
                {user.guest ? <Badge tone="info">{copy.guest}</Badge> : null}
              </h2>
              <p className="truncate text-body-sm text-fg-3">{user.email}</p>
            </div>
          </AvatarEditor>
          <DescriptionList
            columns={2}
            items={[
              { label: copy.job, value: user.title || copy.unset },
              { label: copy.department, value: user.department || copy.unset },
              { label: copy.location, value: user.location || copy.unset },
              ...attributes
                .filter((a) => a.selfService || user.attributes[a.name] !== undefined)
                .map((a) => ({ label: a.label, value: user.attributes[a.name] === undefined ? copy.unset : formatAttribute(a, user.attributes[a.name], locale) })),
              ...(user.guest && user.expiresAt
                ? [
                    { label: copy.sponsor, value: user.sponsor?.name ?? copy.none },
                    { label: copy.accessEnds, value: formatDateTime(user.expiresAt, locale) },
                  ]
                : []),
            ]}
          />
          {editable.length > 0 ? <MyAttributesButton locale={locale} attributes={editable} values={user.attributes} /> : null}
          {user.guest ? (
            <p className="text-body-sm text-fg-2">
              {copy.guestNotice(org.name, user.sponsor?.name)}
            </p>
          ) : null}
          <p className="border-t border-border pt-4 text-caption text-fg-3">
            {copy.profileManaged(org.name)}{" "}
            <a href="mailto:it@example.com" className="text-link underline underline-offset-3 hover:text-ember-hover">
              it@example.com
            </a>
            .
          </p>
        </Card>

        {passwordSession ? null : (
          <Card className="flex flex-col gap-4 p-6 sm:flex-row sm:items-center">
            <span className="grid size-10 shrink-0 place-items-center rounded-md border border-border bg-sunken">
              <ShieldCheck aria-hidden="true" size={20} strokeWidth={1.75} className={resistant ? "text-success" : "text-warning"} />
            </span>
            <div className="flex min-w-0 flex-1 flex-col gap-1">
              <h2 className="text-h4 text-fg">
                {resistant ? copy.usesPasskey : copy.missingPasskey} {copy.strength[user.strength]}.
              </h2>
              <p className="text-body-sm text-fg-3">
                {copy.enrolledMethods(count("passkey"), count("security-key"))}{" "}
                {strong > 1
                  ? copy.multipleMethods
                  : strong === 1
                    ? copy.singleMethod
                    : copy.noMethods}
              </p>
            </div>
            <Link href="/account/security" className="shrink-0 text-body-sm text-link underline underline-offset-3 hover:text-ember-hover">
              {copy.reviewSecurity}
            </Link>
          </Card>
        )}

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

        <Card className="flex flex-col gap-4 p-6 sm:flex-row sm:items-center">
          <span className="grid size-10 shrink-0 place-items-center rounded-md border border-border bg-sunken">
            <Languages aria-hidden="true" size={20} strokeWidth={1.75} className="text-fg-3" />
          </span>
          <div className="flex min-w-0 flex-1 flex-col gap-1">
            <h2 className="text-h4 text-fg">{copy.language}</h2>
            <p className="text-body-sm text-fg-3">{copy.languageHint}</p>
          </div>
          <div className="shrink-0 sm:w-48">
            <LanguagePicker chosen={user.locale} />
          </div>
        </Card>

        <Card className="flex flex-col gap-4 p-6 sm:flex-row sm:items-center">
          <span className="grid size-10 shrink-0 place-items-center rounded-md border border-border bg-sunken">
            <FileDown aria-hidden="true" size={20} strokeWidth={1.75} className="text-fg-3" />
          </span>
          <div className="flex min-w-0 flex-1 flex-col gap-1">
            <h2 className="text-h4 text-fg">{copy.data}</h2>
            <p className="text-body-sm text-fg-3">{copy.dataHint}</p>
          </div>
          <div className="shrink-0">
            <ExportButton path="/me/export" label={copy.download} variant="secondary" size="sm" />
          </div>
        </Card>
      </div>
    </div>
  );
}
