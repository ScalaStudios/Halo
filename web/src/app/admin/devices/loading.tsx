import { PageSkeleton, TableSkeleton, TabsSkeleton } from "@/components/ui/skeleton";
import { directoryMessages } from "@/lib/i18n/console/directory";
import { frameMessages } from "@/lib/i18n/console/frame";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  const locale = await requestLocale();
  const title = frameMessages[locale].nav.items.devices.label;
  return (
    <PageSkeleton title={title} description={160}>
      <TabsSkeleton labels={[title, directoryMessages[locale].devices.complianceSources]} />
      <TableSkeleton columns={9} views={5} facets={2} rows={10} />
    </PageSkeleton>
  );
}
