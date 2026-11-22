import { PageSkeleton, TableSkeleton } from "@/components/ui/skeleton";
import { frameMessages } from "@/lib/i18n/console/frame";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  return (
    <PageSkeleton title={frameMessages[await requestLocale()].nav.items.applications.label} description={78}>
      <TableSkeleton columns={9} views={5} facets={3} rows={10} />
    </PageSkeleton>
  );
}
