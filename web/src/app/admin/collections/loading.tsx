import { PageSkeleton, TableSkeleton } from "@/components/ui/skeleton";
import { directoryMessages } from "@/lib/i18n/console/directory";
import { frameMessages } from "@/lib/i18n/console/frame";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  const locale = await requestLocale();
  return (
    <PageSkeleton title={frameMessages[locale].nav.items.collections.label} description={directoryMessages[locale].collections.description}>
      <TableSkeleton columns={4} rows={4} />
    </PageSkeleton>
  );
}
