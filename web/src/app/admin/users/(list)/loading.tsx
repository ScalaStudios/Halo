import { PageSkeleton, TableSkeleton } from "@/components/ui/skeleton";
import { frameMessages } from "@/lib/i18n/console/frame";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  return (
    <PageSkeleton title={frameMessages[await requestLocale()].nav.items.users.label} description={22}>
      <TableSkeleton columns={8} views={6} facets={4} rows={10} />
    </PageSkeleton>
  );
}
