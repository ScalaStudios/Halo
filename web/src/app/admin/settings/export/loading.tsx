import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { PageSkeleton, Skeleton, SkeletonText } from "@/components/ui/skeleton";
import { frameMessages } from "@/lib/i18n/console/frame";
import { settingsMessages } from "@/lib/i18n/console/settings";
import { requestLocale } from "@/lib/i18n/server";

function Lists({ counts }: { counts: number[] }) {
  return counts.map((items, i) => (
    <div key={i} aria-hidden="true" className="flex flex-col gap-3">
      <p className="text-label">
        <SkeletonText chars={14} />
      </p>
      <div className="flex flex-col gap-2">
        {Array.from({ length: items }, (_, item) => (
          <p key={item} className="text-body-sm">
            <SkeletonText chars={[48, 40, 34][item % 3]} />
          </p>
        ))}
      </div>
    </div>
  ));
}

export default async function Loading() {
  const locale = await requestLocale();
  const { exportPage: t, scheduledExports } = settingsMessages[locale];
  return (
    <PageSkeleton title={frameMessages[locale].nav.items.export.label} description={t.description} className="max-w-4xl">
      <Card>
        <CardHeader title={t.contains} description={t.containsHint} />
        <CardBody className="grid grid-cols-1 gap-8 md:grid-cols-2">
          <Lists counts={[6, 3]} />
        </CardBody>
      </Card>
      <Skeleton className="h-10 w-44 rounded-md" />
      <Card>
        <CardHeader title={t.import} description={t.importHint} />
        <CardBody className="flex flex-col gap-8">
          <div className="grid grid-cols-1 gap-8 md:grid-cols-2">
            <Lists counts={[4, 3]} />
          </div>
          <Skeleton className="h-10 w-48 rounded-md" />
        </CardBody>
      </Card>
      <Card>
        <CardHeader title={scheduledExports.title} description={scheduledExports.description} />
        <CardBody className="flex flex-col gap-6">
          {["h-10 w-60 rounded-md", "h-10 w-32 rounded-md"].map((size) => (
            <div key={size} aria-hidden="true" className="flex flex-col gap-2">
              <p className="text-label">
                <SkeletonText chars={16} />
              </p>
              <Skeleton className={size} />
            </div>
          ))}
        </CardBody>
      </Card>
    </PageSkeleton>
  );
}
