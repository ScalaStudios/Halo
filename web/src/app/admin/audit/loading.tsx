import { PageSkeleton, TableSkeleton } from "@/components/ui/skeleton";
import { frameMessages } from "@/lib/i18n/console/frame";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  return (
    <PageSkeleton title={frameMessages[await requestLocale()].nav.items.audit.label} description={89}>
      <TableSkeleton columns={6} facets={2} rows={10} />
    </PageSkeleton>
  );
}
