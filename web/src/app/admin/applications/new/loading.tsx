import { Card } from "@/components/ui/card";
import { PageSkeleton, SkeletonText } from "@/components/ui/skeleton";
import { applicationsMessages } from "@/lib/i18n/console/applications";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  const t = applicationsMessages[await requestLocale()];
  return (
    <PageSkeleton
      back={{ href: "/admin/applications", label: t.common.applications }}
      title={t.list.add}
      description={t.list.addDescription}
      className="max-w-3xl"
    >
      <Card>
        <div aria-hidden="true" className="flex gap-6 border-b border-border px-6">
          {[t.setup.steps.protocol, t.setup.steps.platform, t.setup.steps.details].map((label, index) => (
            <span key={label} className="flex h-12 items-center gap-2 text-body-sm font-medium text-fg-3">
              <span className="tnum">{index + 1}</span>
              {label}
            </span>
          ))}
        </div>
        <div className="flex flex-col gap-4 p-6">
          <p className="text-h4">
            <SkeletonText chars={18} />
          </p>
          {[0, 1, 2].map((i) => (
            <div key={i} className="h-20 rounded-md border border-border" />
          ))}
        </div>
      </Card>
    </PageSkeleton>
  );
}
