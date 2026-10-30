import { Card } from "@/components/ui/card";
import { PageSkeleton, Skeleton, SkeletonText } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton title="Access reviews" description={87} actions={<Skeleton className="h-10 w-36 rounded-md" />}>
      <Card>
        <ul aria-hidden="true">
          {[0, 1].map((i) => (
            <li key={i} className="flex flex-col gap-4 border-t border-border p-6 first:border-t-0 lg:flex-row lg:items-center lg:gap-8">
              <div className="flex min-w-0 flex-1 flex-col gap-1">
                <p className="text-h4">
                  <SkeletonText chars={26} />
                </p>
                <p className="text-body-sm">
                  <SkeletonText chars={22} />
                </p>
                <p className="text-caption">
                  <SkeletonText chars={48} />
                </p>
              </div>
              <Skeleton className="h-2 w-full rounded-full lg:w-48" />
              <Skeleton className="h-8 w-28 shrink-0 rounded-md" />
            </li>
          ))}
        </ul>
      </Card>
    </PageSkeleton>
  );
}
