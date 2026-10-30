import { PageSkeleton, TableSkeleton } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton title="Users" description={22}>
      <TableSkeleton columns={8} views={6} facets={4} rows={10} />
    </PageSkeleton>
  );
}
