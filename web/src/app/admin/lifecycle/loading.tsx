import { SectionTitle } from "@/components/ui/section-title";
import { PageSkeleton, TableSkeleton } from "@/components/ui/skeleton";
import { directoryMessages } from "@/lib/i18n/console/directory";
import { frameMessages } from "@/lib/i18n/console/frame";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  const locale = await requestLocale();
  const t = directoryMessages[locale].lifecycle;
  return (
    <PageSkeleton title={frameMessages[locale].nav.items.lifecycle.label} description={120}>
      <section className="flex flex-col gap-4">
        <SectionTitle title={t.rules} description={t.rulesDescription} />
        <TableSkeleton columns={8} facets={2} rows={2} />
      </section>
      <section className="flex flex-col gap-4">
        <SectionTitle title={t.history} description={t.historyDescription} />
        <TableSkeleton columns={7} facets={3} rows={3} />
      </section>
    </PageSkeleton>
  );
}
