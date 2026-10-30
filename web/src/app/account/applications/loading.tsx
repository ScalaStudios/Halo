import { PageSkeleton, Skeleton, SkeletonText } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton large title="Applications" description="Everything you can open with your Halo account.">
      <ul aria-hidden="true" className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3">
        {[14, 10, 12, 16, 9, 13].map((chars, i) => (
          <li key={i} className="flex items-center gap-4 rounded-lg border border-border bg-surface p-4">
            <Skeleton className="size-12 shrink-0 rounded-md" />
            <span className="flex min-w-0 flex-1 flex-col gap-1">
              <span className="text-body-sm">
                <SkeletonText chars={chars} />
              </span>
              <span className="text-caption">
                <SkeletonText chars={18} />
              </span>
            </span>
          </li>
        ))}
      </ul>
    </PageSkeleton>
  );
}
