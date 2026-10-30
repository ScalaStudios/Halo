import { ListSkeleton, PageSkeleton, Skeleton, SkeletonText } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton
      large
      title="Sessions"
      description="Browsers, devices and Halo CLI sign-ins on your Halo account. If you don't recognise one, sign it out and review your security."
    >
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
