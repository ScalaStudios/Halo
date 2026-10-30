import { PageSkeleton, TableSkeleton } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton title="Risk events" description={139}>
      <TableSkeleton columns={9} views={4} facets={2} rows={4} />
    </PageSkeleton>
  );
}
