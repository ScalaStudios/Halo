import { Card, CardHeader } from "@/components/ui/card";
import { ListSkeleton, PageSkeleton, SkeletonText } from "@/components/ui/skeleton";
import { policyMessages } from "@/lib/i18n/console/policy";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  const t = policyMessages[await requestLocale()].recommendations;
  return (
    <PageSkeleton title={t.title} description={160} className="max-w-4xl">
      <Card>
        <CardHeader title={t.needsAction} description={<SkeletonText chars={26} />} />
        <ListSkeleton media className="border-t border-border" />
      </Card>
      <Card>
        <CardHeader title={t.passing} />
        <ListSkeleton media rows={4} className="border-t border-border" />
      </Card>
    </PageSkeleton>
  );
}
