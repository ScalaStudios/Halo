import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { DescriptionList } from "@/components/ui/description-list";
import { SectionTitle } from "@/components/ui/section-title";
import { DetailHeaderSkeleton, PageSkeleton, Skeleton, SkeletonText, TableSkeleton } from "@/components/ui/skeleton";
import { applicationsMessages } from "@/lib/i18n/console/applications";
import { frameMessages } from "@/lib/i18n/console/frame";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  const locale = await requestLocale();
  const t = applicationsMessages[locale];
  const facts = [t.apiResources.accessTokenLifetime, t.common.applications, t.common.created];
  return (
    <PageSkeleton
      back={{ href: "/admin/api-resources", label: frameMessages[locale].nav.items.apiResources.label }}
      heading={
        <>
          <DetailHeaderSkeleton actions={2} />
          <DescriptionList
            className="grid-cols-2 rounded-lg border border-border bg-surface p-6 md:grid-cols-3"
            items={facts.map((label) => ({ label, value: <SkeletonText chars={14} /> }))}
          />
        </>
      }
    >
      <div className="flex flex-col gap-6">
        <SectionTitle title={t.apiResources.scopes} description={t.apiResources.scopesHint} />
        <TableSkeleton simple columns={3} rows={2} />
      </div>
      <div className="flex flex-col gap-6">
        <SectionTitle
          title={t.common.applications}
          description={t.apiResources.grantsHint}
          action={<Skeleton className="h-8 w-32 rounded-md" />}
        />
        <TableSkeleton simple columns={3} rows={1} />
      </div>
      <Card>
        <CardHeader title={t.apiResources.validate} description={t.apiResources.validateHint} />
        <CardBody>
          <div className="h-40 rounded-md border border-border bg-sunken" />
        </CardBody>
      </Card>
    </PageSkeleton>
  );
}
