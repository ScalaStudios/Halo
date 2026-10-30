import { PageSkeleton, TableSkeleton } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton title="Groups" description={82}>
      <TableSkeleton columns={4} facets={2} rows={10} />
    </PageSkeleton>
  );
}
