import { SectionTitle } from "@/components/ui/section-title";
import { ListSkeleton, PageSkeleton } from "@/components/ui/skeleton";
import { accountMessages } from "@/lib/i18n/account";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  const copy = accountMessages(await requestLocale());
  return (
    <PageSkeleton large back={{ href: "/account/groups", label: copy.nav.groups }} description={90}>
      <section className="flex flex-col gap-4">
        <SectionTitle title={copy.groups.members} description={copy.groups.membersHint} />
        <ListSkeleton rows={4} className="overflow-hidden rounded-lg border border-border bg-surface" />
      </section>
    </PageSkeleton>
  );
}
