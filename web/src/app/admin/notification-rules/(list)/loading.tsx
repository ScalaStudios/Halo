import { PageSkeleton, Skeleton, TableSkeleton } from "@/components/ui/skeleton";
import { frameMessages } from "@/lib/i18n/console/frame";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  return (
    <PageSkeleton title={frameMessages[await requestLocale()].nav.items.notificationRules.label} description={140}>
      <div className="flex flex-col gap-4">
        <Skeleton className="ml-auto h-8 w-28 rounded-md" />
        <TableSkeleton simple columns={5} rows={2} />
      </div>
    </PageSkeleton>
  );
}
