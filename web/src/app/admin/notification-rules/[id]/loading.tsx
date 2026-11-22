import { DescriptionList } from "@/components/ui/description-list";
import { SectionTitle } from "@/components/ui/section-title";
import { DetailHeaderSkeleton, PageSkeleton, SkeletonText, TableSkeleton } from "@/components/ui/skeleton";
import { frameMessages } from "@/lib/i18n/console/frame";
import { policyMessages } from "@/lib/i18n/console/policy";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  const locale = await requestLocale();
  const t = policyMessages[locale].notificationRules;
  const facts = [t.events, t.conditions, t.lastMessage, t.ruleId];
  return (
    <PageSkeleton
      back={{ href: "/admin/notification-rules", label: frameMessages[locale].nav.items.notificationRules.label }}
      heading={
        <>
          <DetailHeaderSkeleton media="icon" actions={2} />
          <DescriptionList
            className="grid-cols-2 rounded-lg border border-border bg-surface p-6 md:grid-cols-4"
            items={facts.map((label) => ({ label, value: <SkeletonText chars={14} /> }))}
          />
        </>
      }
    >
      <section className="flex flex-col gap-6">
        <SectionTitle title={t.messages} description={<SkeletonText chars={110} />} />
        <TableSkeleton simple columns={6} rows={3} />
      </section>
    </PageSkeleton>
  );
}
