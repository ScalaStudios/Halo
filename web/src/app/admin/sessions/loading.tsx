import { PageSkeleton, TableSkeleton } from "@/components/ui/skeleton";
import { frameMessages } from "@/lib/i18n/console/frame";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  return (
    <PageSkeleton title={frameMessages[await requestLocale()].nav.items.sessions.label} description={86}>
      <TableSkeleton columns={9} facets={1} rows={10} />
    </PageSkeleton>
  );
}
