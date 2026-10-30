import { PageSkeleton, TableSkeleton } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton title="Service accounts" description={125}>
      <TableSkeleton columns={8} facets={2} rows={3} />
    </PageSkeleton>
  );
}
