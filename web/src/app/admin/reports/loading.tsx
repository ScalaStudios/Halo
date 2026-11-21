import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { PageSkeleton, Skeleton, SkeletonText, TableSkeleton, TabsSkeleton } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton title="Reports" description="Who signs in, with which methods and to which applications. Each report downloads as CSV and is available in the API.">
      <TabsSkeleton labels={["Sign-in methods", "Method usage", "Inactive people", "Application usage"]} />
      <Card>
        <CardHeader title={<SkeletonText chars={18} />} description={<SkeletonText chars={56} />} />
        <CardBody>
          <ul aria-hidden="true" className="flex flex-col gap-4">
            {[0, 1, 2, 3, 4].map((i) => (
              <li key={i} className="flex flex-col gap-2">
                <span className="text-body-sm">
                  <SkeletonText chars={14} />
                </span>
                <Skeleton className="h-2 w-full rounded-full" />
              </li>
            ))}
          </ul>
        </CardBody>
      </Card>
      <TableSkeleton columns={5} rows={6} />
    </PageSkeleton>
  );
}
