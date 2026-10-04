import Link from "next/link";
import { AppWindow, ArrowLeft, UsersRound } from "lucide-react";
import { APP_STATUS, PROTOCOL_LABEL } from "@/components/applications/shared";
import { AddMembers, DeleteGroup, EditGroupButton, GroupMembersTable } from "@/components/directory/group-detail";
import { StatusDot, Tag } from "@/components/ui/badge";
import { DescriptionList } from "@/components/ui/description-list";
import { EmptyState } from "@/components/ui/empty-state";
import { SectionTitle } from "@/components/ui/section-title";
import { SimpleTable } from "@/components/ui/simple-table";
import { apiGet } from "@/lib/api/server";
import { formatDate } from "@/lib/format";
import type { Application, Group, User } from "@/lib/types";

export async function generateMetadata({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return { title: (await apiGet<Group>(`/groups/${encodeURIComponent(id)}`)).name };
}

export default async function GroupPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const path = `/groups/${encodeURIComponent(id)}`;
  const [group, members, applications, users] = await Promise.all([
    apiGet<Group>(path),
    apiGet<User[]>(`${path}/members`),
    apiGet<Application[]>("/applications"),
    apiGet<User[]>("/users"),
  ]);
  const dynamic = group.kind === "dynamic";
  const groupApps = applications.filter((a) => a.groupIds.includes(group.id));
  const candidates = users.filter((u) => u.status !== "deprovisioned" && !members.some((m) => m.id === u.id));

  return (
    <div className="flex animate-page flex-col gap-8">
      <div className="flex flex-col gap-6">
        <Link href="/admin/groups" className="inline-flex w-fit items-center gap-2 rounded-sm text-body-sm text-fg-3 transition-colors duration-fast hover:text-fg">
          <ArrowLeft aria-hidden="true" size={16} strokeWidth={1.75} />
          Groups
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
                  Members follow <Tag>{group.rule}</Tag>
                </p>
              ) : null}
            </div>
          </div>
          <EditGroupButton group={group} />
        </header>
        <DescriptionList
          className="grid-cols-2 rounded-lg border border-border bg-surface p-6 md:grid-cols-3 xl:grid-cols-5"
          items={[
            { label: "Membership", value: dynamic ? "Rule-based" : "Assigned" },
            { label: "Members", value: <span className="tnum">{members.length}</span> },
            { label: "Applications", value: <span className="tnum">{groupApps.length}</span> },
            { label: "Source", value: group.source },
            { label: "Created", value: formatDate(group.createdAt) },
          ]}
        />
      </div>

      <section className="flex flex-col gap-6">
        <SectionTitle
          title={dynamic ? "Matching people" : "Members"}
          description={
            dynamic
              ? "Everyone whose profile matches the rule. Change the rule or a person's title, department or location to change who is included."
              : "People added by hand or by provisioning. They get the group's applications at their next sign-in."
          }
          action={dynamic ? undefined : <AddMembers group={group} people={candidates} />}
        />
        {members.length ? (
          <GroupMembersTable group={group} members={members} />
        ) : (
          <EmptyState
            icon={UsersRound}
            title={dynamic ? "Nobody matches the rule" : "No members yet"}
            description={
              dynamic ? (
                <>
                  People appear here as soon as their profile matches <Tag>{group.rule}</Tag>
                </>
              ) : (
                "Add people to give them this group's applications."
              )
            }
          />
        )}
      </section>

      <section className="flex flex-col gap-6">
        <SectionTitle title="Applications" description={`Members of ${group.name} can sign in to these. Assign the group from an application's Users & groups tab.`} />
        {groupApps.length ? (
          <SimpleTable
            caption="Applications"
            head={["Application", "Protocol", "Status", "Users"]}
            rows={groupApps.map((app) => [
              <Link key="n" href={`/admin/applications/${app.id}`} className="font-medium text-fg hover:underline">
                {app.name}
              </Link>,
              PROTOCOL_LABEL[app.protocol],
              <span key="s" className="inline-flex items-center gap-2">
                <StatusDot tone={APP_STATUS[app.status].tone} />
                {APP_STATUS[app.status].label}
              </span>,
              <span key="u" className="tnum">
                {app.userCount}
              </span>,
            ])}
          />
        ) : (
          <EmptyState icon={AppWindow} title="No applications use this group" description="Assign it from an application's Users & groups tab to give members access." />
        )}
      </section>

      <DeleteGroup group={group} appCount={groupApps.length} />
    </div>
  );
}
