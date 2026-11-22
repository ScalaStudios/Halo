import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { FieldSkeleton, PageSkeleton, Skeleton } from "@/components/ui/skeleton";
import { frameMessages } from "@/lib/i18n/console/frame";
import { settingsMessages } from "@/lib/i18n/console/settings";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  const locale = await requestLocale();
  const t = settingsMessages[locale];
  return (
    <PageSkeleton title={frameMessages[locale].nav.items.branding.label} description={t.branding.description} className="max-w-4xl">
      <Card>
        <CardHeader title={t.logo.title} description={t.logo.description} />
        <CardBody className="flex flex-col gap-4">
          <div className="h-36 rounded-md border border-border bg-sunken" />
          <Skeleton className="h-10 w-36 rounded-md" />
        </CardBody>
      </Card>
      <Card>
        <CardHeader title={t.signInMessage.title} description={t.signInMessage.description} />
        <CardBody>
          <div className="flex max-w-xl flex-col gap-6">
            <FieldSkeleton label={t.signInMessage.label} hint={60} tall />
            <Skeleton className="h-10 w-36 rounded-md" />
          </div>
        </CardBody>
      </Card>
      <Card>
        <CardHeader title={t.theme.title} description={t.theme.description} />
        <CardBody>
          <div className="h-64 rounded-md border border-border bg-sunken" />
        </CardBody>
      </Card>
    </PageSkeleton>
  );
}
