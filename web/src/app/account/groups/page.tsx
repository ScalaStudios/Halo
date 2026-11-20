import type { Metadata } from "next";
import Link from "next/link";
import { JoinGroup } from "@/components/account/group-membership";
import { empty, list, row } from "@/components/account/list";
import { joinNames } from "@/components/governance/shared";
import { Badge } from "@/components/ui/badge";
import { PageHeader } from "@/components/ui/page-header";
import { SectionTitle } from "@/components/ui/section-title";
import { apiGet } from "@/lib/api/server";
import { pluralize } from "@/lib/format";
import { JOIN_POLICY } from "@/lib/labels";
import type { Group, JoinableGroup, OwnedGroup } from "@/lib/types";

export const metadata: Metadata = { title: "Groups" };

export default async function GroupsPage() {
  const [groups, owned, joinable] = await Promise.all([
    apiGet<Group[]>("/me/groups"),
    apiGet<OwnedGroup[]>("/me/owned-groups"),
    apiGet<JoinableGroup[]>("/me/joinable-groups"),
  ]);

  return (
    <div className="flex flex-col gap-12">
      <PageHeader size="large" title="Groups" description="Groups decide which applications you see. Join the groups open to you, and manage the members of groups you own." />

      {owned.length ? (
        <section className="flex flex-col gap-4">
          <SectionTitle title="Groups you own" description="You add and remove members and approve requests to join. Changes are recorded in the audit log." />
          <ul className={list}>
            {owned.map((group) => (
              <li key={group.id} className={row}>
                <div className="flex min-w-0 flex-col gap-1">
                  <p className="flex flex-wrap items-center gap-2 text-body-sm font-semibold text-fg">
                    {group.name}
                    {group.requestCount ? <Badge tone="warning">{pluralize(group.requestCount, "request")}</Badge> : null}
                  </p>
                  <p className="tnum text-body-sm text-fg-3">
                    {pluralize(group.memberCount, "member")} · {JOIN_POLICY[group.joinPolicy].label}
                  </p>
                </div>
                <Link href={`/account/groups/${group.id}`} className="shrink-0 text-body-sm text-link underline underline-offset-3 hover:text-ember-hover">
                  Manage members
                </Link>
              </li>
            ))}
          </ul>
        </section>
      ) : null}

      <section className="flex flex-col gap-4">
        <SectionTitle title="Your groups" description="Assigned groups are managed by their owners. Dynamic groups follow your profile, such as your department." />
        {groups.length ? (
          <ul className={list}>
            {groups.map((group) => (
              <li key={group.id} className={row}>
                <div className="flex min-w-0 flex-col gap-1">
                  <p className="text-body-sm font-semibold text-fg">{group.name}</p>
                  {group.description ? <p className="text-body-sm text-fg-3">{group.description}</p> : null}
                  {group.owners.length ? <p className="text-body-sm text-fg-3">Owners: {joinNames(group.owners)}</p> : null}
                </div>
                <span className="shrink-0 text-caption text-fg-3">{group.kind === "dynamic" ? "Dynamic" : "Assigned"}</span>
              </li>
            ))}
          </ul>
        ) : (
          <p className={empty}>You aren&apos;t in any groups yet.</p>
        )}
      </section>

      <section className="flex flex-col gap-4">
        <SectionTitle title="Groups you can join" description="Open groups add you straight away. For the others, an owner approves your request." />
        {joinable.length ? (
          <ul className={list}>
            {joinable.map((group) => (
              <li key={group.id} className={row}>
                <div className="flex min-w-0 flex-col gap-1">
                  <p className="text-body-sm font-semibold text-fg">{group.name}</p>
                  {group.description ? <p className="text-body-sm text-fg-3">{group.description}</p> : null}
                  <p className="text-body-sm text-fg-3">
                    {group.joinPolicy === "open" ? "Anyone can join" : `Owners approve: ${joinNames(group.owners)}`}
                  </p>
                </div>
                {group.requested ? (
                  <Badge tone="warning" className="shrink-0 self-start">
                    Requested
                  </Badge>
                ) : (
                  <JoinGroup group={group} />
                )}
              </li>
            ))}
          </ul>
        ) : (
          <p className={empty}>No groups are open to join right now. To get into another group, ask its owner or a Halo administrator.</p>
        )}
      </section>
    </div>
  );
}
