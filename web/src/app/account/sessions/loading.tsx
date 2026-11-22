import { ListSkeleton, PageSkeleton, Skeleton, SkeletonText } from "@/components/ui/skeleton";
import { accountMessages } from "@/lib/i18n/account";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  const copy = accountMessages(await requestLocale());
  return (
    <PageSkeleton large title={copy.nav.sessions} description={copy.sessions.intro}>
      <div className="flex flex-col gap-4">
        <ListSkeleton rows={2} media className="overflow-hidden rounded-lg border border-border bg-surface" />
        <div aria-hidden="true" className="flex items-center gap-4 rounded-lg border border-border px-6 py-4">
          <span className="flex min-w-0 flex-1 flex-col gap-1">
            <span className="text-body-sm">
              <SkeletonText chars={24} />
            </span>
            <span className="text-body-sm">
              <SkeletonText chars={50} />
            </span>
          </span>
          <Skeleton className="h-8 w-32 shrink-0 rounded-md" />
        </div>
      </div>
    </PageSkeleton>
  );
}
