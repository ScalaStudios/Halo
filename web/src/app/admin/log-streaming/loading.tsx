import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { FieldSkeleton, PageSkeleton } from "@/components/ui/skeleton";
import { frameMessages } from "@/lib/i18n/console/frame";
import { infrastructureMessages } from "@/lib/i18n/console/infrastructure";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  const locale = await requestLocale();
  const t = infrastructureMessages[locale].logStreaming;
  return (
    <PageSkeleton title={frameMessages[locale].nav.items.logStreaming.label} description={120} className="max-w-4xl">
      <Card>
        <CardHeader title={t.destination} description={t.destinationHint} />
        <CardBody className="flex max-w-xl flex-col gap-6">
          <FieldSkeleton label={t.protocol} hint={50} />
          <FieldSkeleton hint={20} />
        </CardBody>
      </Card>
    </PageSkeleton>
  );
}
