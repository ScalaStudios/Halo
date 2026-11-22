import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { FieldSkeleton, PageSkeleton, Skeleton, TabsSkeleton } from "@/components/ui/skeleton";
import { frameMessages } from "@/lib/i18n/console/frame";
import { settingsMessages } from "@/lib/i18n/console/settings";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  const locale = await requestLocale();
  const t = settingsMessages[locale].emailTemplates;
  return (
    <PageSkeleton title={frameMessages[locale].nav.items.emailTemplates.label} description={t.description} className="max-w-6xl">
      <div className="flex flex-col gap-6">
        <TabsSkeleton labels={["Invitation", "Authentication reset", "Sign-in link", "Sign-in code", "Access request", "Access approved", "Access denied"]} />
        <Card>
          <CardHeader title="Invitation" description="Sent when an administrator invites someone to set up their account." />
          <CardBody>
            <div className="grid grid-cols-1 gap-8 lg:grid-cols-2">
              <div className="flex flex-col gap-6">
                <FieldSkeleton label={t.subject} />
                <FieldSkeleton label={t.body} hint={90} tall />
              </div>
              <Skeleton className="h-80 rounded-lg" />
            </div>
          </CardBody>
        </Card>
      </div>
    </PageSkeleton>
  );
}
