import { PageSkeleton, Skeleton, TableSkeleton } from "@/components/ui/skeleton";
import { frameMessages } from "@/lib/i18n/console/frame";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  return (
    <PageSkeleton title={frameMessages[await requestLocale()].nav.items.apiResources.label} description={156} actions={<Skeleton className="h-8 w-24 rounded-md" />}>
      <TableSkeleton simple columns={5} rows={2} />
    </PageSkeleton>
  );
}
