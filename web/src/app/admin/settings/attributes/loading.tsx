import { PageSkeleton, TableSkeleton } from "@/components/ui/skeleton";
import { frameMessages } from "@/lib/i18n/console/frame";
import { settingsMessages } from "@/lib/i18n/console/settings";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  const locale = await requestLocale();
  return (
    <PageSkeleton title={frameMessages[locale].nav.items.attributes.label} description={settingsMessages[locale].attributes.description}>
      <TableSkeleton columns={4} rows={4} />
    </PageSkeleton>
  );
}
