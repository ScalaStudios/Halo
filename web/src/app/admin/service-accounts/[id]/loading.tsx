import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { DescriptionList } from "@/components/ui/description-list";
import { SectionTitle } from "@/components/ui/section-title";
import { DetailHeaderSkeleton, ListSkeleton, PageSkeleton, SkeletonText, TableSkeleton } from "@/components/ui/skeleton";
import { applicationsMessages } from "@/lib/i18n/console/applications";
import { frameMessages } from "@/lib/i18n/console/frame";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  const locale = await requestLocale();
  const t = applicationsMessages[locale];
  const nav = frameMessages[locale].nav.items;
  const facts = [t.common.status, t.common.owner, t.serviceAccounts.activeKeys, t.serviceAccounts.lastUsed, t.common.created, t.serviceAccounts.id];
  return (
    <PageSkeleton
      back={{ href: "/admin/service-accounts", label: nav.serviceAccounts.label }}
      heading={
        <>
          <DetailHeaderSkeleton media="icon" actions={2} />
          <DescriptionList
            className="grid-cols-2 rounded-lg border border-border bg-surface p-6 md:grid-cols-3 xl:grid-cols-6"
            items={facts.map((label) => ({ label, value: <SkeletonText chars={12} /> }))}
          />
        </>
      }
    >
      <section className="flex flex-col gap-6">
        <SectionTitle title={nav.apiKeys.label} description={t.serviceAccounts.keysHint} />
        <TableSkeleton simple columns={8} rows={2} />
      </section>
      <div className="grid grid-cols-1 gap-6 xl:grid-cols-12">
        <div className="flex min-w-0 flex-col gap-6 xl:col-span-5">
          <Card>
            <CardHeader title={t.serviceAccounts.roles} description={t.serviceAccounts.rolesHint} />
            <CardBody>
              <div className="h-20 rounded-md border border-border bg-sunken" />
            </CardBody>
          </Card>
          <Card>
            <CardHeader title={t.serviceAccounts.rateLimit} description={t.serviceAccounts.rateLimitHint} />
            <CardBody>
              <SkeletonText chars={24} />
            </CardBody>
          </Card>
        </div>
        <Card className="min-w-0 xl:col-span-7">
          <CardHeader title={t.serviceAccounts.activity} description={<SkeletonText chars={50} />} />
          <ListSkeleton className="border-t border-border" />
        </Card>
      </div>
    </PageSkeleton>
  );
}
