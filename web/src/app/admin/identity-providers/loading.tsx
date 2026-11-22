import { Card } from "@/components/ui/card";
import { FieldSkeleton, PageSkeleton, Skeleton, TableSkeleton } from "@/components/ui/skeleton";
import { frameMessages } from "@/lib/i18n/console/frame";
import { settingsMessages } from "@/lib/i18n/console/settings";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  const locale = await requestLocale();
  const t = settingsMessages[locale].providers;
  return (
    <PageSkeleton title={frameMessages[locale].nav.items.identityProviders.label} description={147} actions={<Skeleton className="h-8 w-44 rounded-md" />}>
      <Card className="p-6">
        <FieldSkeleton label={t.callback} hint={t.callbackHint} />
      </Card>
      <TableSkeleton simple columns={6} rows={2} />
    </PageSkeleton>
  );
}
