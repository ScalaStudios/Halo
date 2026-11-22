import { PageSkeleton, TableSkeleton } from "@/components/ui/skeleton";
import { frameMessages } from "@/lib/i18n/console/frame";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  return (
    <PageSkeleton title={frameMessages[await requestLocale()].nav.items.serviceAccounts.label} description={125}>
      <TableSkeleton columns={8} facets={2} rows={3} />
    </PageSkeleton>
  );
}
