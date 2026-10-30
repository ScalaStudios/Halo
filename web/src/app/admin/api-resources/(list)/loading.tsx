import { PageSkeleton, Skeleton, TableSkeleton } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton title="API resources" description={156} actions={<Skeleton className="h-8 w-24 rounded-md" />}>
      <TableSkeleton simple columns={5} rows={2} />
    </PageSkeleton>
  );
}
