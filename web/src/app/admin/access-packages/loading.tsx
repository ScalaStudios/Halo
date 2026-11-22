import { PageSkeleton, TableSkeleton } from "@/components/ui/skeleton";
import { frameMessages } from "@/lib/i18n/console/frame";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  return (
    <PageSkeleton title={frameMessages[await requestLocale()].nav.items.accessPackages.label} description={123}>
      <TableSkeleton columns={8} views={3} facets={1} rows={4} />
    </PageSkeleton>
  );
}
