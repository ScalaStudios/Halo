import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { FieldSkeleton, PageSkeleton, Skeleton } from "@/components/ui/skeleton";
import { frameMessages } from "@/lib/i18n/console/frame";
import { settingsMessages } from "@/lib/i18n/console/settings";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  const locale = await requestLocale();
  const { organization: t, retention } = settingsMessages[locale];
  return (
    <PageSkeleton title={frameMessages[locale].nav.items.organization.label} description={t.description} className="max-w-4xl">
      <Card>
        <CardHeader title={t.title} description={t.titleHint} />
        <CardBody>
          <div className="flex max-w-xl flex-col gap-6">
            <FieldSkeleton label={t.name} hint={110} />
            <FieldSkeleton label={t.email} hint={t.emailHint} />
            <FieldSkeleton label={t.language} hint={t.languageHint} />
            <FieldSkeleton label={t.issuer} hint={t.issuerHint} />
            <Skeleton className="h-10 w-44 rounded-md" />
          </div>
        </CardBody>
      </Card>
      <Card>
        <CardHeader title={retention.title} description={retention.description} />
        <CardBody>
          <div className="flex max-w-xl flex-col gap-6">
            <FieldSkeleton label={retention.signInLogDays.label} hint={90} />
            <FieldSkeleton label={retention.auditLogDays.label} hint={60} />
            <Skeleton className="h-10 w-40 rounded-md" />
          </div>
        </CardBody>
      </Card>
    </PageSkeleton>
  );
}
