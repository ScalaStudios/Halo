import Link from "next/link";
import { ArrowLeft, Bot, TriangleAlert } from "lucide-react";
import { AccountActions, KeyTable, RateLimitCard, RolesCard } from "@/components/provisioning/account-actions";
import { FederatedCredentials } from "@/components/provisioning/federated-credentials";
import { ACCOUNT_STATUS, ActivityList, StatusLabel, activeKeys, lastUsed } from "@/components/provisioning/shared";
import { Badge } from "@/components/ui/badge";
import { Card, CardHeader } from "@/components/ui/card";
import { DescriptionList } from "@/components/ui/description-list";
import { SectionTitle } from "@/components/ui/section-title";
import { apiGet } from "@/lib/api/server";
import { formatDate, formatRelative } from "@/lib/format";
import { applicationsMessages } from "@/lib/i18n/console/applications";
import { frameMessages } from "@/lib/i18n/console/frame";
import { requestLocale } from "@/lib/i18n/server";
import type { ServiceAccount } from "@/lib/provisioning-types";
import type { AuditEvent, Organization, User } from "@/lib/types";

export async function generateMetadata({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return { title: (await apiGet<ServiceAccount>(`/service-accounts/${encodeURIComponent(id)}`)).name };
}

export default async function ServiceAccountPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const [account, users, audit, org, locale] = await Promise.all([
    apiGet<ServiceAccount>(`/service-accounts/${encodeURIComponent(id)}`),
    apiGet<User[]>("/users"),
    apiGet<AuditEvent[]>("/audit"),
    apiGet<Organization>("/organization"),
    requestLocale(),
  ]);
  const t = applicationsMessages[locale];
  const nav = frameMessages[locale].nav.items;
  const owner = users.find((u) => u.id === account.ownerId);
  const names = { ...Object.fromEntries(users.map((u) => [u.id, u.name])), [account.id]: account.name };
  const activity = audit.filter((event) => event.actorId === account.id || event.targetId === account.id).slice(0, 20);
  const used = lastUsed([...account.keys, ...account.federatedCredentials]);
  const status = ACCOUNT_STATUS[account.status];

  return (
    <div className="flex flex-col gap-8">
      <div className="flex flex-col gap-6">
        <Link href="/admin/service-accounts" className="inline-flex w-fit items-center gap-2 rounded-sm text-body-sm text-fg-3 transition-colors duration-fast hover:text-fg">
          <ArrowLeft aria-hidden="true" size={16} strokeWidth={1.75} />
          {nav.serviceAccounts.label}
        </Link>
        <header className="flex flex-col gap-6 lg:flex-row lg:items-start lg:justify-between">
          <div className="flex min-w-0 items-center gap-4">
            <span className="grid size-16 shrink-0 place-items-center rounded-lg border border-border bg-sunken text-fg-3">
              <Bot aria-hidden="true" size={24} strokeWidth={1.75} />
            </span>
            <div className="flex min-w-0 flex-col gap-1">
              <div className="flex flex-wrap items-center gap-3">
                <h1 className="text-h2 text-fg">{account.name}</h1>
                <Badge tone={status.tone} dot>
                  {status.label}
                </Badge>
              </div>
              <p className="text-body text-fg-3">{account.description || t.serviceAccounts.noDescription}</p>
            </div>
          </div>
          <AccountActions account={account} scimUrl={`${org.issuer}/scim/v2`} />
        </header>
        {account.status === "suspended" ? (
          <div role="note" className="flex gap-3 rounded-md border border-warning/20 bg-warning-container p-4">
            <TriangleAlert aria-hidden="true" size={16} strokeWidth={1.75} className="mt-0.5 shrink-0 text-warning" />
            <p className="text-body-sm text-fg-2">{t.serviceAccounts.suspended(account.name)}</p>
          </div>
        ) : null}
        <DescriptionList
          className="grid-cols-2 rounded-lg border border-border bg-surface p-6 md:grid-cols-3 xl:grid-cols-6"
          items={[
            { label: t.common.status, value: <StatusLabel {...status} /> },
            {
              label: t.common.owner,
              value: owner ? (
                <Link href={`/admin/users/${owner.id}`} className="text-link underline underline-offset-3 hover:text-ember">
                  {owner.name}
                </Link>
              ) : (
                "—"
              ),
            },
            { label: t.serviceAccounts.activeKeys, value: <span className="tnum">{activeKeys(account.keys).length}</span> },
            { label: t.serviceAccounts.lastUsed, value: used ? formatRelative(used, locale) : t.common.never },
            { label: t.common.created, value: formatDate(account.createdAt, locale) },
            { label: t.serviceAccounts.id, value: account.id, mono: true },
          ]}
        />
      </div>

      <section className="flex flex-col gap-6">
        <SectionTitle title={nav.apiKeys.label} description={t.serviceAccounts.keysHint} />
        <KeyTable keys={account.keys} accountName={account.name} />
      </section>

      <FederatedCredentials account={account} issuer={org.issuer} />

      <div className="grid grid-cols-1 gap-6 xl:grid-cols-12">
        <div className="flex min-w-0 flex-col gap-6 xl:col-span-5">
          <RolesCard account={account} />
          <RateLimitCard account={account} />
        </div>
        <Card className="min-w-0 xl:col-span-7">
          <CardHeader title={t.serviceAccounts.activity} description={t.serviceAccounts.activityHint(account.name)} />
          <ActivityList events={activity} actorNames={names} empty={t.serviceAccounts.noActivity} />
        </Card>
      </div>
    </div>
  );
}
