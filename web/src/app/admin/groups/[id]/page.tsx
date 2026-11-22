import Link from "next/link";
import { AppWindow, ArrowLeft, UserRoundCog, UsersRound } from "lucide-react";
import { APP_STATUS, statusKey } from "@/components/applications/shared";
import { AddMembers, DeleteGroup, EditGroupButton, EditOwners, GroupMembersTable } from "@/components/directory/group-detail";
import { StatusDot, Tag } from "@/components/ui/badge";
import { DescriptionList } from "@/components/ui/description-list";
import { EmptyState } from "@/components/ui/empty-state";
import { SectionTitle } from "@/components/ui/section-title";
import { SimpleTable } from "@/components/ui/simple-table";
import { apiGet } from "@/lib/api/server";
import { formatDate } from "@/lib/format";
import { applicationsMessages } from "@/lib/i18n/console/applications";
import { directoryMessages } from "@/lib/i18n/console/directory";
import { frameMessages } from "@/lib/i18n/console/frame";
import { requestLocale } from "@/lib/i18n/server";
import type { Application, Group, User } from "@/lib/types";

export async function generateMetadata({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return { title: (await apiGet<Group>(`/groups/${encodeURIComponent(id)}`)).name };
}

export default async function GroupPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const path = `/groups/${encodeURIComponent(id)}`;
  const [group, members, applications, users, locale] = await Promise.all([
    apiGet<Group>(path),
    apiGet<User[]>(`${path}/members`),
    apiGet<Application[]>("/applications"),
    apiGet<User[]>("/users"),
    requestLocale(),
  ]);
  const t = directoryMessages[locale];
  const apps = applicationsMessages[locale];
  const dynamic = group.kind === "dynamic";
  const groupApps = applications.filter((a) => a.groupIds.includes(group.id));
  const active = users.filter((u) => u.status !== "deprovisioned");
  const candidates = active.filter((u) => !members.some((m) => m.id === u.id));

  return (
    <div className="flex flex-col gap-8">
      <div className="flex flex-col gap-6">
        <Link href="/admin/groups" className="inline-flex w-fit items-center gap-2 rounded-sm text-body-sm text-fg-3 transition-colors duration-fast hover:text-fg">
          <ArrowLeft aria-hidden="true" size={16} strokeWidth={1.75} />
          {frameMessages[locale].nav.items.groups.label}
        </Link>
        <header className="flex flex-col gap-6 lg:flex-row lg:items-start lg:justify-between">
          <div className="flex min-w-0 items-center gap-4">
            <span className="grid size-16 shrink-0 place-items-center rounded-lg border border-border bg-sunken text-fg-3">
              <UsersRound aria-hidden="true" size={24} strokeWidth={1.75} />
            </span>
            <div className="flex min-w-0 flex-col gap-1">
              <h1 className="truncate text-h2 text-fg">{group.name}</h1>
              {group.description ? <p className="text-body text-fg-3">{group.description}</p> : null}
              {dynamic ? (
                <p className="flex flex-wrap items-center gap-2 text-body-sm text-fg-3">
                  {t.group.membersFollow} <Tag>{group.rule}</Tag>
                </p>
              ) : null}
            </div>
          </div>
          <EditGroupButton group={group} />
        </header>
        <DescriptionList
          className="grid-cols-2 rounded-lg border border-border bg-surface p-6 md:grid-cols-3 xl:grid-cols-6"
          items={[
            { label: t.fields.membership, value: t.groupKind[group.kind] },
            { label: t.group.whoCanJoin, value: dynamic ? t.group.followsRule : t.joinPolicy[group.joinPolicy].label },
            { label: t.fields.members, value: <span className="tnum">{members.length}</span> },
            { label: t.fields.applications, value: <span className="tnum">{groupApps.length}</span> },
            { label: t.fields.source, value: group.source },
            { label: t.fields.created, value: formatDate(group.createdAt, locale) },
          ]}
        />
      </div>

      {dynamic ? null : (
        <section className="flex flex-col gap-6">
          <SectionTitle
            title={t.group.owners}
            description={t.group.ownersDescription}
            action={<EditOwners group={group} people={active} />}
          />
          {group.owners.length ? (
            <SimpleTable
              caption={t.group.owners}
              head={[t.fields.name, t.fields.email]}
              rows={group.owners.map((owner) => [
                <Link key="n" href={`/admin/users/${owner.id}`} className="font-medium text-fg hover:underline">
                  {owner.name}
                </Link>,
                owner.email,
              ])}
            />
          ) : (
            <EmptyState icon={UserRoundCog} title={t.group.noOwners} description={t.group.noOwnersDescription} />
          )}
        </section>
      )}

      <section className="flex flex-col gap-6">
        <SectionTitle
          title={dynamic ? t.group.matchingPeople : t.fields.members}
          description={dynamic ? t.group.matchingDescription : t.group.membersDescription}
          action={dynamic ? undefined : <AddMembers group={group} people={candidates} />}
        />
        {members.length ? (
          <GroupMembersTable group={group} members={members} />
        ) : (
          <EmptyState
            icon={UsersRound}
            title={dynamic ? t.group.nobodyMatchesRule : t.group.noMembers}
            description={
              dynamic ? (
                <>
                  {t.group.appearWhenMatching} <Tag>{group.rule}</Tag>
                </>
              ) : (
                t.group.noMembersDescription
              )
            }
          />
        )}
      </section>

      <section className="flex flex-col gap-6">
        <SectionTitle title={t.fields.applications} description={t.group.applicationsDescription(group.name)} />
        {groupApps.length ? (
          <SimpleTable
            caption={t.fields.applications}
            head={[t.fields.application, t.fields.protocol, t.fields.status, t.fields.users]}
            rows={groupApps.map((app) => [
              <Link key="n" href={`/admin/applications/${app.id}`} className="font-medium text-fg hover:underline">
                {app.name}
              </Link>,
              apps.protocols[app.protocol],
              <span key="s" className="inline-flex items-center gap-2">
                <StatusDot tone={APP_STATUS[statusKey(app)]} />
                {apps.status[statusKey(app)]}
              </span>,
              <span key="u" className="tnum">
                {app.userCount}
              </span>,
            ])}
          />
        ) : (
          <EmptyState icon={AppWindow} title={t.group.noApplications} description={t.group.noApplicationsDescription} />
        )}
      </section>

      <DeleteGroup group={group} appCount={groupApps.length} />
    </div>
  );
}
