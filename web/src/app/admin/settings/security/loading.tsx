import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { ListSkeleton, PageSkeleton, Skeleton, SkeletonText } from "@/components/ui/skeleton";
import { frameMessages } from "@/lib/i18n/console/frame";
import { settingsMessages } from "@/lib/i18n/console/settings";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  const locale = await requestLocale();
  const t = settingsMessages[locale].signingKeys;
  return (
    <PageSkeleton title={frameMessages[locale].nav.items.security.label} description={150} className="max-w-4xl">
      {[1, 2, 1, 3, 2].map((fields, i) => (
        <Card key={i}>
          <CardHeader title={<SkeletonText chars={12} />} description={<SkeletonText chars={80} />} />
          <CardBody className="flex flex-wrap gap-6">
            {Array.from({ length: fields }, (_, field) => (
              <div key={field} aria-hidden="true" className="flex w-60 flex-col gap-2">
                <p className="text-label">
                  <SkeletonText chars={22} />
                </p>
                <Skeleton className="h-10 w-32 rounded-md" />
                <p className="text-caption">
                  <SkeletonText chars={60} />
                </p>
              </div>
            ))}
          </CardBody>
        </Card>
      ))}
      <Card>
        <CardHeader title={t.title} description={t.description} />
        <ListSkeleton rows={2} className="border-t border-border" />
      </Card>
    </PageSkeleton>
  );
}
