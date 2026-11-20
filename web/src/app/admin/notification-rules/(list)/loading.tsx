import { PageSkeleton, Skeleton, TableSkeleton } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton title="Notification rules" description={140}>
      <div className="flex flex-col gap-4">
        <Skeleton className="ml-auto h-8 w-28 rounded-md" />
        <TableSkeleton simple columns={5} rows={2} />
      </div>
    </PageSkeleton>
  );
}
