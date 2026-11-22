import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { DescriptionList } from "@/components/ui/description-list";
import { SectionTitle } from "@/components/ui/section-title";
import { DetailHeaderSkeleton, PageSkeleton, SkeletonText, TableSkeleton } from "@/components/ui/skeleton";
import { frameMessages } from "@/lib/i18n/console/frame";
import { infrastructureMessages } from "@/lib/i18n/console/infrastructure";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  const locale = await requestLocale();
  const t = infrastructureMessages[locale];
  const facts = [t.webhooks.events, t.webhooks.lastDelivery, t.created, t.webhooks.endpointId];
  return (
    <PageSkeleton
      back={{ href: "/admin/webhooks", label: frameMessages[locale].nav.items.webhooks.label }}
      heading={
        <>
          <DetailHeaderSkeleton media="icon" actions={2} />
          <DescriptionList
            className="grid-cols-2 rounded-lg border border-border bg-surface p-6 md:grid-cols-4"
            items={facts.map((label) => ({ label, value: <SkeletonText chars={14} /> }))}
          />
        </>
      }
    >
      <section className="flex flex-col gap-6">
        <SectionTitle title={t.webhooks.deliveries} description={<SkeletonText chars={110} />} />
        <TableSkeleton simple columns={6} rows={3} />
      </section>
      <Card>
        <CardHeader
          title={t.webhooks.verify}
          description={t.webhooks.verifyHint}
        />
        <CardBody>
          <div className="h-96 rounded-md border border-border bg-sunken" />
        </CardBody>
      </Card>
    </PageSkeleton>
  );
}
