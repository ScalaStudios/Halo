import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { DescriptionList } from "@/components/ui/description-list";
import { SectionTitle } from "@/components/ui/section-title";
import { FieldSkeleton, PageSkeleton, Skeleton, SkeletonText, TableSkeleton } from "@/components/ui/skeleton";
import { frameMessages } from "@/lib/i18n/console/frame";
import { infrastructureMessages } from "@/lib/i18n/console/infrastructure";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  const locale = await requestLocale();
  const t = infrastructureMessages[locale];
  return (
    <PageSkeleton title={frameMessages[locale].nav.items.infrastructure.label} description={110}>
      <Card>
        <CardHeader title={t.page.authority} description={t.page.authorityHint} />
        <CardBody className="flex flex-col gap-6">
          <FieldSkeleton label={t.page.caPublicKey} hint={60} />
          <DescriptionList
            columns={2}
            items={[
              { label: t.fingerprint, value: <SkeletonText chars={52} /> },
              { label: t.created, value: <SkeletonText chars={14} /> },
            ]}
          />
          <FieldSkeleton label={t.ssh.lifetime} hint={70} />
          <div className="h-16 rounded-md border border-border bg-sunken" />
        </CardBody>
      </Card>
      <Card>
        <CardHeader
          title={t.page.trustGuide}
          description={t.page.trustGuideHint}
        />
        <CardBody>
          <div className="h-36 rounded-md border border-border bg-sunken" />
        </CardBody>
      </Card>
      <div className="flex flex-col gap-6">
        <SectionTitle
          title={t.ssh.mappings}
          description={t.ssh.mappingsHint}
          action={<Skeleton className="h-8 w-32 rounded-md" />}
        />
        <TableSkeleton simple columns={4} rows={2} />
      </div>
      <div className="flex flex-col gap-6">
        <SectionTitle
          title={t.page.issued}
          description={t.page.issuedHint}
        />
        <TableSkeleton simple columns={6} rows={3} />
      </div>
    </PageSkeleton>
  );
}
