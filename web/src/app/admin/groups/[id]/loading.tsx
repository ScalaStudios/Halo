import { DescriptionList } from "@/components/ui/description-list";
import { SectionTitle } from "@/components/ui/section-title";
import { DetailHeaderSkeleton, PageSkeleton, SkeletonText, TableSkeleton } from "@/components/ui/skeleton";
import { directoryMessages } from "@/lib/i18n/console/directory";
import { frameMessages } from "@/lib/i18n/console/frame";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  const locale = await requestLocale();
  const t = directoryMessages[locale];
  const facts = [t.fields.membership, t.group.whoCanJoin, t.fields.members, t.fields.applications, t.fields.source, t.fields.created];
  return (
    <PageSkeleton
      back={{ href: "/admin/groups", label: frameMessages[locale].nav.items.groups.label }}
      heading={
        <>
          <DetailHeaderSkeleton media="icon" />
          <DescriptionList
            className="grid-cols-2 rounded-lg border border-border bg-surface p-6 md:grid-cols-3 xl:grid-cols-6"
            items={facts.map((label) => ({ label, value: <SkeletonText chars={12} /> }))}
          />
        </>
      }
    >
      <section className="flex flex-col gap-6">
        <SectionTitle title={<SkeletonText chars={10} />} description={<SkeletonText chars={150} />} />
        <TableSkeleton columns={6} facets={3} rows={8} />
      </section>
    </PageSkeleton>
  );
}
