import { SectionTitle } from "@/components/ui/section-title";
import { PageSkeleton, Skeleton, TableSkeleton } from "@/components/ui/skeleton";
import { frameMessages } from "@/lib/i18n/console/frame";
import { settingsMessages } from "@/lib/i18n/console/settings";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  const locale = await requestLocale();
  const t = settingsMessages[locale].domains;
  return (
    <PageSkeleton title={frameMessages[locale].nav.items.domains.label} description={120} className="max-w-4xl">
      <div className="flex flex-col gap-4">
        <SectionTitle title={t.verifiedTitle} description={t.verifiedDescription} action={<Skeleton className="h-8 w-32 rounded-md" />} />
        <TableSkeleton simple columns={5} rows={2} />
      </div>
    </PageSkeleton>
  );
}
