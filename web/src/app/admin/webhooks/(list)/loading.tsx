import { PageSkeleton, Skeleton, TableSkeleton } from "@/components/ui/skeleton";
import { frameMessages } from "@/lib/i18n/console/frame";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  return (
    <PageSkeleton title={frameMessages[await requestLocale()].nav.items.webhooks.label} description={120}>
      <div className="flex flex-col gap-4">
        <Skeleton className="ml-auto h-8 w-32 rounded-md" />
        <TableSkeleton simple columns={4} rows={2} />
      </div>
    </PageSkeleton>
  );
}
