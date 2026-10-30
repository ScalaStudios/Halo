import { PageSkeleton, TableSkeleton } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton title="Access packages" description={123}>
      <TableSkeleton columns={8} views={3} facets={1} rows={4} />
    </PageSkeleton>
  );
}
