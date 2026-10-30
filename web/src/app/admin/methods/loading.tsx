import { Card } from "@/components/ui/card";
import { PageSkeleton, Skeleton, SkeletonText } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton title="Methods" description={125} className="max-w-4xl">
      <Card>
        <ul aria-hidden="true">
          {[14, 12, 18, 10, 14, 8].map((chars, i) => (
            <li key={i} className="flex flex-col gap-4 border-b border-border px-4 py-6 last:border-b-0 sm:flex-row sm:items-start sm:justify-between sm:gap-8 sm:px-6">
              <div className="flex min-w-0 flex-1 gap-4">
                <Skeleton className="size-10 shrink-0 rounded-md" />
                <div className="flex min-w-0 flex-1 flex-col gap-1">
                  <p className="text-body">
                    <SkeletonText chars={chars} />
                  </p>
                  <p className="text-body-sm">
                    <SkeletonText chars={110} />
                  </p>
                  <p className="text-body-sm">
                    <SkeletonText chars={34} />
                  </p>
                </div>
              </div>
              <Skeleton className="h-5 w-20 shrink-0 rounded-sm" />
            </li>
          ))}
        </ul>
      </Card>
    </PageSkeleton>
  );
}
