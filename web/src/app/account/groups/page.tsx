import type { Metadata } from "next";
import Link from "next/link";
import { JoinGroup } from "@/components/account/group-membership";
import { empty, list, row } from "@/components/account/list";
import { joinNames } from "@/components/governance/shared";
import { Badge } from "@/components/ui/badge";
import { PageHeader } from "@/components/ui/page-header";
import { SectionTitle } from "@/components/ui/section-title";
import { apiGet } from "@/lib/api/server";
import { accountMessages } from "@/lib/i18n/account";
import { directoryMessages } from "@/lib/i18n/console/directory";
import { requestLocale } from "@/lib/i18n/server";
import type { Group, JoinableGroup, OwnedGroup } from "@/lib/types";

export async function generateMetadata(): Promise<Metadata> {
  return { title: accountMessages(await requestLocale()).nav.groups };
}

export default async function GroupsPage() {
  const [groups, owned, joinable, locale] = await Promise.all([
    apiGet<Group[]>("/me/groups"),
    apiGet<OwnedGroup[]>("/me/owned-groups"),
    apiGet<JoinableGroup[]>("/me/joinable-groups"),
    requestLocale(),
  ]);
  const copy = accountMessages(locale);
  const t = copy.groups;
  const policies = directoryMessages[locale].joinPolicy;

  return (
    <div className="flex flex-col gap-12">
      <PageHeader size="large" title={copy.nav.groups} description={t.description} />

      {owned.length ? (
        <section className="flex flex-col gap-4">
          <SectionTitle title={t.owned} description={t.ownedHint} />
          <ul className={list}>
            {owned.map((group) => (
              <li key={group.id} className={row}>
                <div className="flex min-w-0 flex-col gap-1">
                  <p className="flex flex-wrap items-center gap-2 text-body-sm font-semibold text-fg">
                    {group.name}
                    {group.requestCount ? <Badge tone="warning">{t.requestCount(group.requestCount)}</Badge> : null}
                  </p>
                  <p className="tnum text-body-sm text-fg-3">
                    {t.memberCount(group.memberCount)} · {policies[group.joinPolicy].label}
                  </p>
                </div>
                <Link href={`/account/groups/${group.id}`} className="shrink-0 text-body-sm text-link underline underline-offset-3 hover:text-ember-hover">
                  {t.manage}
                </Link>
              </li>
            ))}
          </ul>
        </section>
      ) : null}

      <section className="flex flex-col gap-4">
        <SectionTitle title={t.yours} description={t.yoursHint} />
        {groups.length ? (
          <ul className={list}>
            {groups.map((group) => (
              <li key={group.id} className={row}>
                <div className="flex min-w-0 flex-col gap-1">
                  <p className="text-body-sm font-semibold text-fg">{group.name}</p>
                  {group.description ? <p className="text-body-sm text-fg-3">{group.description}</p> : null}
                  {group.owners.length ? <p className="text-body-sm text-fg-3">{t.owners(joinNames(group.owners, locale))}</p> : null}
                </div>
                <span className="shrink-0 text-caption text-fg-3">{group.kind === "dynamic" ? t.dynamic : t.assigned}</span>
              </li>
            ))}
          </ul>
        ) : (
          <p className={empty}>{t.none}</p>
        )}
      </section>

      <section className="flex flex-col gap-4">
        <SectionTitle title={t.joinable} description={t.joinableHint} />
        {joinable.length ? (
          <ul className={list}>
            {joinable.map((group) => (
              <li key={group.id} className={row}>
                <div className="flex min-w-0 flex-col gap-1">
                  <p className="text-body-sm font-semibold text-fg">{group.name}</p>
                  {group.description ? <p className="text-body-sm text-fg-3">{group.description}</p> : null}
                  <p className="text-body-sm text-fg-3">
                    {group.joinPolicy === "open" ? t.anyone : t.ownersApprove(joinNames(group.owners, locale))}
                  </p>
                </div>
                {group.requested ? (
                  <Badge tone="warning" className="shrink-0 self-start">
                    {t.requested}
                  </Badge>
                ) : (
                  <JoinGroup group={group} />
                )}
              </li>
            ))}
          </ul>
        ) : (
          <p className={empty}>{t.noneJoinable}</p>
        )}
      </section>
    </div>
  );
}
