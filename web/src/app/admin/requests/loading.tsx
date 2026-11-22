import { Card, CardHeader } from "@/components/ui/card";
import { SectionTitle } from "@/components/ui/section-title";
import { ListSkeleton, PageSkeleton, SkeletonText, TableSkeleton } from "@/components/ui/skeleton";
import { frameMessages } from "@/lib/i18n/console/frame";
import { governanceMessages } from "@/lib/i18n/console/governance";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  const locale = await requestLocale();
  const t = governanceMessages[locale];
  return (
    <PageSkeleton title={frameMessages[locale].nav.items.requests.label} description={126}>
      <Card>
        <CardHeader title={t.pending} description={<SkeletonText chars={12} />} />
        <ListSkeleton media className="pb-2" />
      </Card>
      <section className="flex flex-col gap-4">
        <SectionTitle title={t.decided} description={t.pages.requestsDecided} />
        <TableSkeleton columns={5} facets={2} rows={4} />
      </section>
    </PageSkeleton>
  );
}
