import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { PageSkeleton, Skeleton, SkeletonText, TableSkeleton, TabsSkeleton } from "@/components/ui/skeleton";
import { frameMessages } from "@/lib/i18n/console/frame";
import { governanceMessages } from "@/lib/i18n/console/governance";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  const locale = await requestLocale();
  const t = governanceMessages[locale].reports;
  return (
    <PageSkeleton title={frameMessages[locale].nav.items.reports.label} description={t.description}>
      <TabsSkeleton labels={Object.values(t.tabs)} />
      <Card>
        <CardHeader title={<SkeletonText chars={18} />} description={<SkeletonText chars={56} />} />
        <CardBody>
          <ul aria-hidden="true" className="flex flex-col gap-4">
            {[0, 1, 2, 3, 4].map((i) => (
              <li key={i} className="flex flex-col gap-2">
                <span className="text-body-sm">
                  <SkeletonText chars={14} />
                </span>
                <Skeleton className="h-2 w-full rounded-full" />
              </li>
            ))}
          </ul>
        </CardBody>
      </Card>
      <TableSkeleton columns={5} rows={6} />
    </PageSkeleton>
  );
}
