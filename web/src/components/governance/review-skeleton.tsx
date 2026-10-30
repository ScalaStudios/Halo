import { Card } from "@/components/ui/card";
import { DescriptionList } from "@/components/ui/description-list";
import { Skeleton, SkeletonText, TableSkeleton } from "@/components/ui/skeleton";

export function ReviewSkeleton() {
  return (
    <>
      <Card className="p-6">
        <DescriptionList columns={2} items={["Group", "Reviewers", "Due", "Automatic removal", "Started"].map((label) => ({ label, value: <SkeletonText chars={22} /> }))} />
      </Card>
      <Card className="flex flex-col gap-4 p-6 sm:flex-row sm:items-center">
        <Skeleton className="h-2 w-full rounded-full sm:w-48" />
        <p className="flex-1 text-body-sm">
          <SkeletonText chars={90} />
        </p>
        <Skeleton className="h-10 w-40 rounded-md" />
      </Card>
      <TableSkeleton columns={4} views={3} facets={1} rows={6} />
    </>
  );
}
