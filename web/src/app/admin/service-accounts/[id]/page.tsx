import Link from "next/link";
import { ArrowLeft, Bot, TriangleAlert } from "lucide-react";
import { AccountActions, KeyTable, RolesCard } from "@/components/provisioning/account-actions";
import { ACCOUNT_STATUS, ActivityList, StatusLabel, activeKeys, lastUsed } from "@/components/provisioning/shared";
import { Badge } from "@/components/ui/badge";
import { Card, CardHeader } from "@/components/ui/card";
import { DescriptionList } from "@/components/ui/description-list";
import { SectionTitle } from "@/components/ui/section-title";
import { apiGet } from "@/lib/api/server";
import { formatDate, formatRelative } from "@/lib/format";
import type { ServiceAccount } from "@/lib/provisioning-types";
import type { AuditEvent, Organization, User } from "@/lib/types";

export async function generateMetadata({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return { title: (await apiGet<ServiceAccount>(`/service-accounts/${encodeURIComponent(id)}`)).name };
}

export default async function ServiceAccountPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const [account, users, audit, org] = await Promise.all([
    apiGet<ServiceAccount>(`/service-accounts/${encodeURIComponent(id)}`),
    apiGet<User[]>("/users"),
    apiGet<AuditEvent[]>("/audit"),
    apiGet<Organization>("/organization"),
  ]);
  const owner = users.find((u) => u.id === account.ownerId);
  const names = { ...Object.fromEntries(users.map((u) => [u.id, u.name])), [account.id]: account.name };
  const activity = audit.filter((event) => event.actorId === account.id || event.targetId === account.id).slice(0, 20);
  const used = lastUsed(account.keys);
  const status = ACCOUNT_STATUS[account.status];

  return (
    <div className="flex animate-page flex-col gap-8">
      <div className="flex flex-col gap-6">
        <Link href="/admin/service-accounts" className="inline-flex w-fit items-center gap-2 rounded-sm text-body-sm text-fg-3 transition-colors duration-fast hover:text-fg">
          <ArrowLeft aria-hidden="true" size={16} strokeWidth={1.75} />
          Service accounts
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
              <p className="text-body text-fg-3">{account.description || "Service account without a description."}</p>
            </div>
          </div>
          <AccountActions account={account} scimUrl={`${org.issuer}/scim/v2`} />
        </header>
        {account.status === "suspended" ? (
          <div role="note" className="flex gap-3 rounded-md border border-warning/20 bg-warning-container p-4">
            <TriangleAlert aria-hidden="true" size={16} strokeWidth={1.75} className="mt-0.5 shrink-0 text-warning" />
            <p className="text-body-sm text-fg-2">{account.name} is disabled, so requests with its API keys are rejected. Enable it from the actions menu to restore access.</p>
          </div>
        ) : null}
        <DescriptionList
          className="grid-cols-2 rounded-lg border border-border bg-surface p-6 md:grid-cols-3 xl:grid-cols-6"
          items={[
            { label: "Status", value: <StatusLabel {...status} /> },
            {
              label: "Owner",
              value: owner ? (
                <Link href={`/admin/users/${owner.id}`} className="text-link underline underline-offset-3 hover:text-ember">
                  {owner.name}
                </Link>
              ) : (
                "—"
              ),
            },
            { label: "Active keys", value: <span className="tnum">{activeKeys(account.keys).length}</span> },
            { label: "Last used", value: used ? formatRelative(used) : "Never" },
            { label: "Created", value: formatDate(account.createdAt) },
            { label: "Service account ID", value: account.id, mono: true },
          ]}
        />
      </div>

      <section className="flex flex-col gap-6">
        <SectionTitle title="API keys" description="Send a key as Authorization: Bearer <key>. The api scope opens the management API; the scim scope opens SCIM provisioning." />
        <KeyTable keys={account.keys} accountName={account.name} />
      </section>

      <div className="grid grid-cols-1 gap-6 xl:grid-cols-12">
        <div className="min-w-0 xl:col-span-5">
          <RolesCard account={account} />
        </div>
        <Card className="min-w-0 xl:col-span-7">
          <CardHeader title="Activity" description={`Changes made by or to ${account.name}, newest first.`} />
          <ActivityList events={activity} actorNames={names} empty="No changes recorded yet." />
        </Card>
      </div>
    </div>
  );
}
