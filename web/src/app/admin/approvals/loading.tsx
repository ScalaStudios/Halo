import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { SectionTitle } from "@/components/ui/section-title";
import { FieldSkeleton, ListSkeleton, PageSkeleton, Skeleton, SkeletonText, TableSkeleton } from "@/components/ui/skeleton";
import { frameMessages } from "@/lib/i18n/console/frame";
import { governanceMessages } from "@/lib/i18n/console/governance";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  const locale = await requestLocale();
  const t = governanceMessages[locale];
  return (
    <PageSkeleton title={frameMessages[locale].nav.items.approvals.label} description={140}>
      <Card>
        <CardHeader title={t.pending} description={<SkeletonText chars={12} />} />
        <ListSkeleton media className="pb-2" />
      </Card>
      <Card>
        <CardHeader title={t.pages.approvalSettings} description={t.pages.approvalSettingsHint} />
        <CardBody className="flex max-w-xl flex-col gap-6">
          <div aria-hidden="true" className="flex flex-col gap-4">
            {[38, 34, 28, 30].map((chars) => (
              <div key={chars} className="flex items-start gap-3">
                <Skeleton className="mt-0.5 size-4 rounded-sm" />
                <span className="flex flex-col">
                  <span className="text-body-sm">
                    <SkeletonText chars={chars} />
                  </span>
                  <span className="text-caption">
                    <SkeletonText chars={chars + 24} />
                  </span>
                </span>
              </div>
            ))}
          </div>
          <FieldSkeleton label={t.pages.emergencyAccounts} hint={140} />
        </CardBody>
      </Card>
      <section className="flex flex-col gap-4">
        <SectionTitle title={t.decided} description={t.pages.approvalsDecided} />
        <TableSkeleton columns={4} facets={1} rows={4} />
      </section>
    </PageSkeleton>
  );
}
