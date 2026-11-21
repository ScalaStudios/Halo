import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { PageSkeleton, Skeleton, SkeletonText, TableSkeleton } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton title="Risk events" description={139}>
      <TableSkeleton columns={9} views={4} facets={2} rows={4} />
      <Card>
        <CardHeader title="Automatic responses" description={<SkeletonText chars={160} />} />
        <CardBody>
          <Skeleton className="h-10 w-full max-w-3xl rounded-md" />
        </CardBody>
      </Card>
    </PageSkeleton>
  );
}
