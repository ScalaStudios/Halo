import { PageSkeleton, TableSkeleton } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton title="Sessions" description={86}>
      <TableSkeleton columns={9} facets={1} rows={10} />
    </PageSkeleton>
  );
}
