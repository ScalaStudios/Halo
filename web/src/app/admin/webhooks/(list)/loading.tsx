import { PageSkeleton, Skeleton, TableSkeleton } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton title="Webhooks" description={120}>
      <div className="flex flex-col gap-4">
        <Skeleton className="ml-auto h-8 w-32 rounded-md" />
        <TableSkeleton simple columns={4} rows={2} />
      </div>
    </PageSkeleton>
  );
}
