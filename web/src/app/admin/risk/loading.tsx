import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { PageSkeleton, Skeleton, SkeletonText, TableSkeleton } from "@/components/ui/skeleton";
import { frameMessages } from "@/lib/i18n/console/frame";
import { policyMessages } from "@/lib/i18n/console/policy";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  const locale = await requestLocale();
  return (
    <PageSkeleton title={frameMessages[locale].nav.items.risk.label} description={139}>
      <TableSkeleton columns={9} views={4} facets={2} rows={4} />
      <Card>
        <CardHeader title={policyMessages[locale].responses.title} description={<SkeletonText chars={160} />} />
        <CardBody>
          <Skeleton className="h-10 w-full max-w-3xl rounded-md" />
        </CardBody>
      </Card>
    </PageSkeleton>
  );
}
