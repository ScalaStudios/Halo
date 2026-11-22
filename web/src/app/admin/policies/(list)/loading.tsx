import { Card, CardHeader } from "@/components/ui/card";
import { ListSkeleton, PageSkeleton, Skeleton, TabsSkeleton } from "@/components/ui/skeleton";
import { frameMessages } from "@/lib/i18n/console/frame";
import { policyMessages } from "@/lib/i18n/console/policy";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  const locale = await requestLocale();
  const t = policyMessages[locale];
  return (
    <PageSkeleton
      title={frameMessages[locale].nav.items.policies.label}
      description={140}
      actions={
        <>
          <Skeleton className="h-8 w-36 rounded-md" />
          <Skeleton className="h-8 w-32 rounded-md" />
        </>
      }
    >
      <TabsSkeleton labels={[t.page.tabs.policies, t.page.tabs.networks, t.page.tabs.terms]} />
      <Card>
        <CardHeader title={t.list.insights} description={t.list.insightsDescription} />
        <ListSkeleton rows={2} className="border-t border-border" />
      </Card>
      <Card>
        <ListSkeleton />
      </Card>
    </PageSkeleton>
  );
}
