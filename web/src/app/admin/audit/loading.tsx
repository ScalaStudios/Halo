import { PageSkeleton, TableSkeleton } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton title="Audit log" description={89}>
      <TableSkeleton columns={6} facets={2} rows={10} />
    </PageSkeleton>
  );
}
