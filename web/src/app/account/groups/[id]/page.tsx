import Link from "next/link";
import { ArrowLeft } from "lucide-react";
import { JoinRequests, OwnedMembers } from "@/components/account/group-membership";
import { empty } from "@/components/account/list";
import { AddMembers } from "@/components/directory/group-detail";
import { PageHeader } from "@/components/ui/page-header";
import { SectionTitle } from "@/components/ui/section-title";
import { apiGet } from "@/lib/api/server";
import { accountMessages } from "@/lib/i18n/account";
import { directoryMessages } from "@/lib/i18n/console/directory";
import { requestLocale } from "@/lib/i18n/server";
import type { OwnedGroupDetail } from "@/lib/types";

type Params = Promise<{ id: string }>;

export async function generateMetadata({ params }: { params: Params }) {
  const { id } = await params;
  return { title: (await apiGet<OwnedGroupDetail>(`/me/owned-groups/${encodeURIComponent(id)}`)).group.name };
}

export default async function OwnedGroupPage({ params }: { params: Params }) {
  const { id } = await params;
  const [{ group, members, requests, people }, locale] = await Promise.all([apiGet<OwnedGroupDetail>(`/me/owned-groups/${encodeURIComponent(id)}`), requestLocale()]);
  const copy = accountMessages(locale);
  const t = copy.groups;
  const policy = directoryMessages[locale].joinPolicy[group.joinPolicy];

  return (
    <div className="flex flex-col gap-12">
      <div className="flex flex-col gap-6">
        <Link href="/account/groups" className="inline-flex w-fit items-center gap-2 rounded-sm text-body-sm text-fg-3 transition-colors duration-fast hover:text-fg">
          <ArrowLeft aria-hidden="true" size={16} strokeWidth={1.75} />
          {copy.nav.groups}
        </Link>
        <PageHeader
          size="large"
          title={group.name}
          description={[group.description, `${policy.label}: ${policy.description}`].filter(Boolean).join(" · ")}
        />
      </div>

      {requests.length ? (
        <section className="flex flex-col gap-4">
          <SectionTitle title={t.joinRequests} description={t.joinRequestsHint} />
          <JoinRequests groupId={group.id} groupName={group.name} requests={requests} />
        </section>
      ) : null}

      <section className="flex flex-col gap-4">
        <SectionTitle
          title={t.members}
          description={t.membersHint}
          action={<AddMembers group={group} people={people} />}
        />
        {members.length ? (
          <OwnedMembers groupId={group.id} groupName={group.name} members={members} />
        ) : (
          <p className={empty}>{t.noMembers}</p>
        )}
      </section>
    </div>
  );
}
