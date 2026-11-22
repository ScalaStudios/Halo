import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { SectionTitle } from "@/components/ui/section-title";
import { PageSkeleton, Skeleton, TableSkeleton } from "@/components/ui/skeleton";
import { frameMessages } from "@/lib/i18n/console/frame";
import { remoteMessages } from "@/lib/i18n/console/infrastructure/remote";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  const locale = await requestLocale();
  const t = remoteMessages[locale];
  return (
    <PageSkeleton title={frameMessages[locale].nav.items.remoteAccess.label} description={150}>
      <div className="flex flex-col gap-6">
        <SectionTitle
          title={t.outposts.title}
          description={t.outposts.hint}
          action={<Skeleton className="h-8 w-36 rounded-md" />}
        />
        <TableSkeleton simple columns={5} rows={2} />
      </div>
      <Card>
        <CardHeader
          title={t.page.guide}
          description={t.page.guideHint}
        />
        <CardBody>
          <div className="h-40 rounded-md border border-border bg-sunken" />
        </CardBody>
      </Card>
      <div className="flex flex-col gap-6">
        <SectionTitle
          title={t.page.sessions}
          description={t.page.sessionsHint}
        />
        <TableSkeleton simple columns={6} rows={4} />
      </div>
    </PageSkeleton>
  );
}
